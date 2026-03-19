package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// StartDevServer reads the surface contract, discovers Blocks, and starts an HTTP server.
// This is the development adapter — it bridges Surface HTTP requests to Block execution.
func StartDevServer(projectRoot string, rootDomain *DomainYaml, port int) error {
	// Find surface.yaml
	surfacePath, err := findSurface(projectRoot)
	if err != nil {
		// No surface found — run in direct mode (expose all Blocks)
		fmt.Fprintf(os.Stderr, "[aglet serve] No surface.yaml found — exposing all Blocks as /block/{name}\n")
		return startDirectServer(projectRoot, rootDomain, port)
	}

	// Parse the surface contract and resolve surface metadata
	deps, err := parseSurfaceContract(surfacePath)
	if err != nil {
		return fmt.Errorf("failed to parse surface contract: %w", err)
	}

	// Resolve the surface directory and name for contract call logging.
	// The wrapper uses this context to write contract.call entries to the
	// surface's logs.jsonl whenever a contract endpoint is called.
	surfaceDir := filepath.Dir(surfacePath)
	surfaceName := parseSurfaceName(surfacePath)

	fmt.Fprintf(os.Stderr, "[aglet serve] Parsed contract from: %s\n", surfacePath)

	// Build the block graph for pipeline resolution
	graph, err := BuildBlockGraph(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to build block graph: %w", err)
	}

	// Discover all Blocks for listing
	blocks := discoverAllBlocks(projectRoot)

	mux := http.NewServeMux()

	// Register contract endpoints
	for depName, dep := range deps {
		depName := depName
		dep := dep

		if dep.Block != "" {
			// Single Block endpoint
			_, err := FindBlock(projectRoot, dep.Block)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  POST /contract/%s → Block '%s' (NOT FOUND)\n", depName, dep.Block)
				continue
			}
			fmt.Fprintf(os.Stderr, "  POST /contract/%s → Block '%s'\n", depName, dep.Block)
			contractName := depName // capture for closure
			mux.HandleFunc("/contract/"+depName, func(w http.ResponseWriter, r *http.Request) {
				handleContractBlockRequest(w, r, dep.Block, projectRoot, rootDomain, surfaceDir, surfaceName, contractName)
			})

		} else if dep.Pipeline != "" {
			// Pipeline endpoint — resolve calls graph from the start Block
			pipeline, err := graph.FindPipelineFrom(dep.Pipeline)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  POST /contract/%s → Pipeline from '%s' (ERROR: %s)\n", depName, dep.Pipeline, err)
				continue
			}
			fmt.Fprintf(os.Stderr, "  POST /contract/%s → Pipeline: %s\n", depName, strings.Join(pipeline, " → "))
			mux.HandleFunc("/contract/"+depName, func(w http.ResponseWriter, r *http.Request) {
				handlePipelineRequest(w, r, pipeline, projectRoot, rootDomain)
			})

		} else {
			// No explicit Block or Pipeline — try name as Block
			_, err := FindBlock(projectRoot, depName)
			if err == nil {
				fmt.Fprintf(os.Stderr, "  POST /contract/%s → Block '%s'\n", depName, depName)
				contractName := depName // capture for closure
				mux.HandleFunc("/contract/"+depName, func(w http.ResponseWriter, r *http.Request) {
					handleContractBlockRequest(w, r, depName, projectRoot, rootDomain, surfaceDir, surfaceName, contractName)
				})
			} else {
				fmt.Fprintf(os.Stderr, "  POST /contract/%s → (no matching Block)\n", depName)
			}
		}
	}

	// Expose direct Block access — the primary dev interface
	mux.HandleFunc("/block/", func(w http.ResponseWriter, r *http.Request) {
		blockName := strings.TrimPrefix(r.URL.Path, "/block/")
		if blockName == "" {
			http.Error(w, `{"error": "block name required"}`, http.StatusBadRequest)
			return
		}
		handleBlockRequest(w, r, blockName, projectRoot, rootDomain)
	})

	// SDK interaction events endpoint — receives batched client-side events
	// from the @aglet/sdk and appends them to the surface's logs.jsonl.
	agletSurfaceDir := ResolveAgletDirForSurface(surfaceDir, surfaceName, projectRoot)
	mux.HandleFunc("/_aglet/events", func(w http.ResponseWriter, r *http.Request) {
		handleInteractionEvents(w, r, agletSurfaceDir)
	})

	// CORS middleware for local development
	handler := corsMiddleware(mux)

	fmt.Fprintf(os.Stderr, "\n[aglet serve] Dev server running on http://localhost:%d\n", port)
	fmt.Fprintf(os.Stderr, "  Contract endpoints: /contract/{DependencyName}\n")
	fmt.Fprintf(os.Stderr, "  Direct access:      /block/{BlockName}\n")
	fmt.Fprintf(os.Stderr, "  SDK events:         /_aglet/events\n")
	if len(blocks) > 0 {
		fmt.Fprintf(os.Stderr, "\n  Available Blocks:\n")
		for _, b := range blocks {
			fmt.Fprintf(os.Stderr, "    /block/%s  (%s)\n", b.Config.Name, b.Config.Runtime)
		}
	}
	fmt.Fprintf(os.Stderr, "\n")

	return http.ListenAndServe(fmt.Sprintf(":%d", port), handler)
}

// startDirectServer exposes all Blocks as /block/{name} without a contract.
func startDirectServer(projectRoot string, rootDomain *DomainYaml, port int) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/block/", func(w http.ResponseWriter, r *http.Request) {
		blockName := strings.TrimPrefix(r.URL.Path, "/block/")
		if blockName == "" {
			http.Error(w, `{"error": "block name required"}`, http.StatusBadRequest)
			return
		}
		handleBlockRequest(w, r, blockName, projectRoot, rootDomain)
	})

	handler := corsMiddleware(mux)

	fmt.Fprintf(os.Stderr, "\n[aglet serve] Dev server running on http://localhost:%d\n", port)
	fmt.Fprintf(os.Stderr, "  Direct access: /block/{BlockName}\n\n")

	return http.ListenAndServe(fmt.Sprintf(":%d", port), handler)
}

// handleContractBlockRequest executes a single Block via a surface contract endpoint.
// It extracts surface context from headers (X-Aglet-Caller) and passes it through
// to the block wrapper, which writes a contract.call entry to the surface's logs.jsonl.
func handleContractBlockRequest(w http.ResponseWriter, r *http.Request, blockName string, projectRoot string, rootDomain *DomainYaml, surfaceDir string, surfaceName string, contractName string) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	input := readHTTPInput(r)

	block, err := FindBlock(projectRoot, blockName)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Block not found: %s"}`, err), http.StatusNotFound)
		return
	}

	// Build surface context from HTTP headers and server state.
	// The wrapper uses this to write contract.call entries to the surface's logs.
	caller := r.Header.Get("X-Aglet-Caller")
	surfaceCtx := &SurfaceCallContext{
		SurfaceDir:      surfaceDir,
		SurfaceName:     surfaceName,
		Caller:          caller,
		Contract:        contractName,
		AgletSurfaceDir: ResolveAgletDirForSurface(surfaceDir, surfaceName, projectRoot),
	}

	opts := WrapBlockOptions{
		ForwardCalls:   true,
		SurfaceContext: surfaceCtx,
	}

	output, err := WrapBlockWithOptions(block, rootDomain, projectRoot, input, opts)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Block execution failed: %s"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(output)
}

// handleBlockRequest executes a single Block and returns its output as HTTP response.
// Used for direct /block/{name} access — no surface context.
func handleBlockRequest(w http.ResponseWriter, r *http.Request, blockName string, projectRoot string, rootDomain *DomainYaml) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	input := readHTTPInput(r)

	block, err := FindBlock(projectRoot, blockName)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Block not found: %s"}`, err), http.StatusNotFound)
		return
	}

	output, err := dispatchBlock(block, rootDomain, projectRoot, input)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Block execution failed: %s"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(output)
}

// handlePipelineRequest executes a pipeline of Blocks and returns the final output.
func handlePipelineRequest(w http.ResponseWriter, r *http.Request, pipeline []string, projectRoot string, rootDomain *DomainYaml) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	input := readHTTPInput(r)

	output, err := RunPipeline(pipeline, projectRoot, rootDomain, input)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Pipeline execution failed: %s"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(output)
}

// readHTTPInput reads JSON input from the request body or defaults to empty JSON.
func readHTTPInput(r *http.Request) []byte {
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			return []byte("{}")
		}
		defer r.Body.Close()
		return data
	}
	return []byte("{}")
}

// findSurface searches for a surface.yaml file in the project.
func findSurface(projectRoot string) (string, error) {
	var surfacePath string

	err := filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.Name() == "surface.yaml" {
			surfacePath = path
			return filepath.SkipAll
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	if surfacePath == "" {
		return "", fmt.Errorf("no surface.yaml found in project")
	}
	return surfacePath, nil
}

// parseSurfaceContract reads a surface.yaml file and extracts the contract dependencies.
func parseSurfaceContract(path string) (map[string]ContractDependency, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var surface SurfaceYaml
	if err := yaml.Unmarshal(data, &surface); err != nil {
		return nil, fmt.Errorf("invalid surface.yaml: %w", err)
	}
	return surface.Contract.Dependencies, nil
}

// handleInteractionEvents receives batched client-side interaction events from
// the @aglet/sdk and appends them to the surface's logs.jsonl. The SDK buffers
// events in the browser and flushes them periodically (every 5 min by default)
// and on page unload via sendBeacon.
//
// Request body is a JSON array of interaction events:
//
//	[{"event":"interaction","timestamp":"...","caller":"FeedbackPanel","surface":"Dashboard","action":"button_click"}]
func handleInteractionEvents(w http.ResponseWriter, r *http.Request, agletSurfaceDir string) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, `{"error": "POST required"}`, http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error": "failed to read body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse the batch — array of interaction events
	var events []map[string]interface{}
	if err := json.Unmarshal(body, &events); err != nil {
		http.Error(w, `{"error": "invalid JSON array"}`, http.StatusBadRequest)
		return
	}

	// Append each event to the surface's .aglet/ logs
	EnsureAgletDir(agletSurfaceDir)
	logPath := filepath.Join(agletSurfaceDir, "logs.jsonl")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "could not open log: %s"}`, err), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	for _, event := range events {
		// Ensure source is marked as "sdk" so we can distinguish from
		// server-side events in the logs
		event["source"] = "sdk"
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		f.Write(data)
		f.WriteString("\n")
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok": true}`))
}

// parseSurfaceName reads the surface name from a surface.yaml file.
// Falls back to the directory name if the name field isn't set.
func parseSurfaceName(surfacePath string) string {
	data, err := os.ReadFile(surfacePath)
	if err != nil {
		return filepath.Base(filepath.Dir(surfacePath))
	}
	var surface SurfaceYaml
	if err := yaml.Unmarshal(data, &surface); err != nil || surface.Name == "" {
		return filepath.Base(filepath.Dir(surfacePath))
	}
	return surface.Name
}

// corsMiddleware adds CORS headers for local development.
// Allows X-Aglet-Caller and X-Aglet-Surface headers for surface contract observability.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Aglet-Caller, X-Aglet-Surface")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// discoverAllBlocks scans the project for all block.yaml files and returns them.
func discoverAllBlocks(projectRoot string) []*DiscoveredBlock {
	var blocks []*DiscoveredBlock

	filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.Name() != "block.yaml" {
			return nil
		}
		block, err := ParseBlockDir(filepath.Dir(path))
		if err != nil {
			return nil
		}
		if block.Config.Runtime != "embedded" {
			blocks = append(blocks, block)
		}
		return nil
	})

	return blocks
}
