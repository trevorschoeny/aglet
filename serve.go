package main

import (
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

	// Parse the surface contract
	deps, err := parseSurfaceContract(surfacePath)
	if err != nil {
		return fmt.Errorf("failed to parse surface contract: %w", err)
	}

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
			mux.HandleFunc("/contract/"+depName, func(w http.ResponseWriter, r *http.Request) {
				handleBlockRequest(w, r, dep.Block, projectRoot, rootDomain)
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
				mux.HandleFunc("/contract/"+depName, func(w http.ResponseWriter, r *http.Request) {
					handleBlockRequest(w, r, depName, projectRoot, rootDomain)
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

	// CORS middleware for local development
	handler := corsMiddleware(mux)

	fmt.Fprintf(os.Stderr, "\n[aglet serve] Dev server running on http://localhost:%d\n", port)
	fmt.Fprintf(os.Stderr, "  Contract endpoints: /contract/{DependencyName}\n")
	fmt.Fprintf(os.Stderr, "  Direct access:      /block/{BlockName}\n")
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

// handleBlockRequest executes a single Block and returns its output as HTTP response.
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

// corsMiddleware adds CORS headers for local development.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

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
