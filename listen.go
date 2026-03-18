package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// StartDomainListener starts an HTTP server for a single domain.
// It discovers all blocks within the domain and exposes them as HTTP endpoints.
// This is the domain listener — the per-domain router that accepts requests
// from the outside world and forwards them to block wrappers.
//
// The listener does NOT execute blocks directly. It routes requests to
// WrapBlock, which handles observability and execution.
//
// Routes:
//   POST /block/{BlockName}  — execute a block by name
//
// The listener reads X-Aglet-Caller and X-Aglet-Surface headers for
// surface observability context (which component triggered the call).
func StartDomainListener(domainDir string, rootDomain *DomainYaml, projectRoot string, port int) error {
	// Discover all non-embedded blocks within this domain
	blocks := discoverDomainBlocks(domainDir)

	mux := http.NewServeMux()

	// Register block endpoints
	mux.HandleFunc("/block/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		blockName := strings.TrimPrefix(r.URL.Path, "/block/")
		if blockName == "" {
			http.Error(w, `{"error": "block name required — use /block/{BlockName}"}`, http.StatusBadRequest)
			return
		}

		// Read input from request body
		input := readListenerInput(r)

		// Find the block — first look in this domain, then project-wide
		block, err := FindBlock(projectRoot, blockName)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Block not found: %s"}`, err), http.StatusNotFound)
			return
		}

		// Read surface observability context from headers
		// (logged by the wrapper as part of the execution context)
		caller := r.Header.Get("X-Aglet-Caller")
		surface := r.Header.Get("X-Aglet-Surface")
		if caller != "" || surface != "" {
			// TODO: Pass caller/surface context through to wrapper for
			// surface-level logging. For now, these headers are accepted
			// but the surface logging integration happens in a later phase.
			_ = caller
			_ = surface
		}

		// Execute through the wrapper — full observability
		output, err := WrapBlock(block, rootDomain, projectRoot, input)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Block execution failed: %s"}`, err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(output)
	})

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","domain":"%s"}`, rootDomain.Name)
	})

	// CORS middleware for development
	handler := corsMiddleware(mux)

	// Print startup info
	fmt.Fprintf(os.Stderr, "[aglet listen] Domain: %s\n", rootDomain.Name)
	fmt.Fprintf(os.Stderr, "[aglet listen] Listening on http://localhost:%d\n", port)
	fmt.Fprintf(os.Stderr, "  Block endpoint: POST /block/{BlockName}\n")
	if len(blocks) > 0 {
		fmt.Fprintf(os.Stderr, "\n  Available Blocks:\n")
		for _, b := range blocks {
			fmt.Fprintf(os.Stderr, "    /block/%s  (%s)\n", b.Config.Name, b.Config.Runtime)
		}
	}
	if len(rootDomain.Peers) > 0 {
		fmt.Fprintf(os.Stderr, "\n  Peers:\n")
		for name, url := range rootDomain.Peers {
			fmt.Fprintf(os.Stderr, "    %s → %s\n", name, url)
		}
	}
	fmt.Fprintf(os.Stderr, "\n")

	return http.ListenAndServe(fmt.Sprintf(":%d", port), handler)
}

// discoverDomainBlocks finds all blocks within a domain directory.
// Only returns non-embedded blocks (embedded blocks can't be executed externally).
func discoverDomainBlocks(domainDir string) []*DiscoveredBlock {
	var blocks []*DiscoveredBlock

	filepath.Walk(domainDir, func(path string, info os.FileInfo, err error) error {
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

// readListenerInput reads JSON input from the HTTP request body.
func readListenerInput(r *http.Request) []byte {
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

// handleListen is the CLI handler for `aglet listen`.
// Starts a domain listener for the current directory's domain.
func handleListen() {
	// Parse flags
	port := 3001
	for i := 2; i < len(os.Args)-1; i++ {
		if os.Args[i] == "--port" {
			fmt.Sscanf(os.Args[i+1], "%d", &port)
		}
	}

	// Find the domain — look in cwd first, then walk up
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot get working directory: %s\n", err)
		os.Exit(1)
	}

	// Find domain.yaml in current directory or parent directories
	domainDir, rootDomain, err := findDomainForListen(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	// Find project root (needed for cross-domain block resolution)
	projectRoot, err := findProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	if err := StartDomainListener(domainDir, rootDomain, projectRoot, port); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

// findDomainForListen finds the nearest domain.yaml starting from dir.
func findDomainForListen(dir string) (string, *DomainYaml, error) {
	current := dir
	for {
		domainPath := filepath.Join(current, "domain.yaml")
		if _, err := os.Stat(domainPath); err == nil {
			domain, err := ParseDomainYaml(domainPath)
			if err != nil {
				return "", nil, fmt.Errorf("error parsing %s: %w", domainPath, err)
			}
			return current, domain, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return "", nil, fmt.Errorf("no domain.yaml found — run this from within a domain directory")
}
