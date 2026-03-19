package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
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

	// --- Surface detection ---
	// Look for a surface.yaml within this domain. If found, the listener
	// becomes the single entry point: it proxies the frontend dev server,
	// injects SDK config into HTML, and handles contract + block endpoints.
	surfacePath, _ := findSurface(domainDir)
	var surface *SurfaceYaml
	var surfaceDir string
	var devProxy *httputil.ReverseProxy
	var sdkConfigScript string

	if surfacePath != "" {
		s, err := parseSurfaceYaml(surfacePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[aglet listen] Warning: could not parse surface.yaml: %s\n", err)
		} else {
			surface = s
			surfaceDir = filepath.Dir(surfacePath)

			// Build the SDK config script that will be injected into HTML
			sdkConfigScript = buildSDKConfigScript(surface)

			// Start the surface's dev server if a dev command is configured
			if surface.Dev.Command != "" && surface.Dev.Port > 0 {
				devCmd, err := startDevServer(surface, surfaceDir)
				if err != nil {
					fmt.Fprintf(os.Stderr, "[aglet listen] Warning: could not start dev server: %s\n", err)
				} else {
					// Wait briefly for the dev server to start
					waitForDevServer(surface.Dev.Port)

					// Create a reverse proxy to the dev server
					devURL, _ := url.Parse(fmt.Sprintf("http://localhost:%d", surface.Dev.Port))
					devProxy = httputil.NewSingleHostReverseProxy(devURL)

					// Intercept HTML responses to inject the SDK config
					devProxy.ModifyResponse = func(resp *http.Response) error {
						return injectSDKConfig(resp, sdkConfigScript)
					}

					fmt.Fprintf(os.Stderr, "[aglet listen] Surface: %s (proxying dev server on port %d)\n", surface.Name, surface.Dev.Port)

					// Clean up the dev server when the listener exits
					defer devCmd.Process.Kill()
				}
			}
		}
	}

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

		input := readListenerInput(r)

		block, err := FindBlock(projectRoot, blockName)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Block not found: %s"}`, err), http.StatusNotFound)
			return
		}

		// Build wrapper options with surface context if available
		opts := DefaultWrapOptions()
		if surface != nil {
			caller := r.Header.Get("X-Aglet-Caller")
			opts.SurfaceContext = &SurfaceCallContext{
				SurfaceDir:      surfaceDir,
				SurfaceName:     surface.Name,
				Caller:          caller,
				Contract:        blockName,
				AgletSurfaceDir: ResolveAgletDirForSurface(surfaceDir, surface.Name, projectRoot),
			}
		}

		output, err := WrapBlockWithOptions(block, rootDomain, projectRoot, input, opts)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Block execution failed: %s"}`, err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(output)
	})

	// Register contract endpoints if a surface was found
	if surface != nil {
		for depName, dep := range surface.Contract.Dependencies {
			depName := depName
			dep := dep

			// Resolve the block name: prefer block field, fall back to pipeline
			// field (which names the first block in a pipeline chain), then the
			// contract dependency name itself.
			blockName := dep.Block
			if blockName == "" {
				blockName = dep.Pipeline
			}
			if blockName == "" {
				blockName = depName
			}

			if _, err := FindBlock(projectRoot, blockName); err != nil {
				fmt.Fprintf(os.Stderr, "  POST /contract/%s → Block '%s' (NOT FOUND)\n", depName, blockName)
				continue
			}

			fmt.Fprintf(os.Stderr, "  POST /contract/%s → Block '%s'\n", depName, blockName)
			contractName := depName
			mux.HandleFunc("/contract/"+depName, func(w http.ResponseWriter, r *http.Request) {
				handleContractBlockRequest(w, r, blockName, projectRoot, rootDomain, surfaceDir, surface.Name, contractName)
			})
		}

		// SDK interaction events endpoint
		agletSurfaceDir := ResolveAgletDirForSurface(surfaceDir, surface.Name, projectRoot)
		mux.HandleFunc("/_aglet/events", func(w http.ResponseWriter, r *http.Request) {
			handleInteractionEvents(w, r, agletSurfaceDir)
		})

		// Simple JSON store for surface data persistence
		mux.HandleFunc("/_aglet/store", func(w http.ResponseWriter, r *http.Request) {
			handleSurfaceStore(w, r, agletSurfaceDir)
		})
	}

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","domain":"%s"}`, rootDomain.Name)
	})

	// If we have a dev proxy, add a catch-all that forwards unmatched
	// requests to the frontend dev server (Vite, Next, etc.)
	if devProxy != nil {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			devProxy.ServeHTTP(w, r)
		})
	}

	handler := corsMiddleware(mux)

	// Print startup info
	fmt.Fprintf(os.Stderr, "\n[aglet listen] Domain: %s\n", rootDomain.Name)
	fmt.Fprintf(os.Stderr, "[aglet listen] Listening on http://localhost:%d\n", port)
	fmt.Fprintf(os.Stderr, "  Block endpoint:    POST /block/{BlockName}\n")
	if surface != nil {
		fmt.Fprintf(os.Stderr, "  Contract endpoint: POST /contract/{Name}\n")
		fmt.Fprintf(os.Stderr, "  SDK events:        POST /_aglet/events\n")
	}
	if devProxy != nil {
		fmt.Fprintf(os.Stderr, "  Frontend proxy:    → localhost:%d\n", surface.Dev.Port)
		fmt.Fprintf(os.Stderr, "  SDK config:        injected into HTML responses\n")
	}
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

// parseSurfaceYaml reads and parses a surface.yaml file.
func parseSurfaceYaml(path string) (*SurfaceYaml, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var surface SurfaceYaml
	if err := yaml.Unmarshal(data, &surface); err != nil {
		return nil, err
	}
	return &surface, nil
}

// buildSDKConfigScript creates the <script> tag content that will be injected
// into HTML responses. This is how surface.yaml config reaches the SDK in the
// browser — the listener reads the yaml, the browser reads the global.
func buildSDKConfigScript(surface *SurfaceYaml) string {
	config := map[string]interface{}{
		"surface": surface.Name,
	}

	// Apply SDK config from surface.yaml, with defaults
	flushInterval := 300
	if surface.SDK.FlushInterval > 0 {
		flushInterval = surface.SDK.FlushInterval
	}
	config["flushInterval"] = flushInterval

	trackInteractions := true
	if surface.SDK.TrackInteractions != nil {
		trackInteractions = *surface.SDK.TrackInteractions
	}
	config["trackInteractions"] = trackInteractions

	data, _ := json.Marshal(config)
	return fmt.Sprintf(`<script>window.__AGLET__=%s;</script>`, string(data))
}

// startDevServer starts the surface's frontend dev server as a child process.
// The command comes from surface.yaml's dev.command field.
func startDevServer(surface *SurfaceYaml, surfaceDir string) (*exec.Cmd, error) {
	parts := strings.Fields(surface.Dev.Command)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty dev command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = surfaceDir
	cmd.Stdout = os.Stderr // Dev server output goes to stderr (same as aglet's own output)
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PORT=%d", surface.Dev.Port), // Some frameworks respect PORT env var
	)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start dev server: %w", err)
	}

	fmt.Fprintf(os.Stderr, "[aglet listen] Starting dev server: %s (port %d)\n", surface.Dev.Command, surface.Dev.Port)
	return cmd, nil
}

// waitForDevServer polls the dev server port until it responds or times out.
func waitForDevServer(port int) {
	addr := fmt.Sprintf("http://localhost:%d", port)
	for i := 0; i < 30; i++ {
		resp, err := http.Get(addr)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Fprintf(os.Stderr, "[aglet listen] Warning: dev server on port %d not responding after 15s — proxying anyway\n", port)
}

// injectSDKConfig intercepts HTML responses from the dev server and injects
// the SDK config script tag before </head>. This is how surface.yaml config
// reaches the @aglet/sdk running in the browser.
func injectSDKConfig(resp *http.Response, scriptTag string) error {
	// Only inject into HTML responses
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		return nil
	}

	// Read the entire response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	resp.Body.Close()

	// Inject the script tag before </head> (or at the end if no </head> found)
	injected := body
	headClose := []byte("</head>")
	if idx := bytes.Index(bytes.ToLower(body), headClose); idx >= 0 {
		// Insert the script tag right before </head>
		injected = make([]byte, 0, len(body)+len(scriptTag))
		injected = append(injected, body[:idx]...)
		injected = append(injected, []byte(scriptTag)...)
		injected = append(injected, body[idx:]...)
	} else {
		// No </head> found — append to the end
		injected = append(body, []byte(scriptTag)...)
	}

	// Replace the response body and update Content-Length
	resp.Body = io.NopCloser(bytes.NewReader(injected))
	resp.ContentLength = int64(len(injected))
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(injected)))

	return nil
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
