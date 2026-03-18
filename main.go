package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Support --version and -v as top-level flags before the command switch
	if os.Args[1] == "--version" || os.Args[1] == "-v" {
		handleVersion()
		return
	}

	switch os.Args[1] {
	case "run":
		handleRun()
	case "reason":
		handleReason()
	case "pipe":
		handlePipe()
	case "listen":
		handleListen()
	case "serve":
		// Deprecated: aglet serve is replaced by aglet listen (per-domain listener).
		// Keep it working for backward compatibility with a deprecation warning.
		fmt.Fprintf(os.Stderr, "[aglet] Warning: 'aglet serve' is deprecated — use 'aglet listen' instead.\n")
		fmt.Fprintf(os.Stderr, "         'aglet listen' runs a per-domain listener (same binary, dev/prod parity).\n\n")
		handleServe()
	case "validate":
		handleValidate()
	case "init":
		handleInit()
	case "new":
		handleNew()
	case "stats":
		handleStats()
	case "version":
		handleVersion()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `aglet — development toolkit for the Aglet protocol

Commands:
  run <BlockName> [input.json]          Find and execute a Block by name
  reason <BlockDir> [input.json]        Execute a reasoning Block directly from its directory
  pipe <StartBlock> [EndBlock]          Execute a pipeline by following calls edges
  listen [--port PORT]                  Start a per-domain listener (replaces serve)
  serve [--port PORT]                   (deprecated — use listen)
  init <ProjectName> [flags]            Bootstrap a new Aglet project
  new <type> <name> [flags]             Scaffold a new Block, Domain, Surface, or Component
  stats [BlockName] [--domain D] [--project] [--write] [--json]  Behavioral memory from logs
  validate [--deep] [--unit NAME] [--json]  Validate project structure and consistency
  version                               Print the aglet version

`)
}

// handleVersion prints the installed version of aglet.
// When installed via `go install github.com/trevorschoeny/aglet@vX.Y.Z`, Go embeds
// the module version automatically — no build flags needed.
// In local/dev builds (go run ., go build without a tag), it prints "(dev build)".
func handleVersion() {
	version := "dev"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	fmt.Printf("aglet %s\n", version)
}

// handleRun finds a Block by name in the project and executes it.
// This is the discovery-based path — it scans block.yaml files to locate the Block.
func handleRun() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: aglet run <BlockName> [input.json]\n")
		os.Exit(1)
	}

	blockName := os.Args[2]
	inputFile := ""
	if len(os.Args) > 3 {
		inputFile = os.Args[3]
	}

	// Find the project root by walking up from cwd looking for a root domain.yaml
	projectRoot, err := findProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	// Discover the Block
	block, err := FindBlock(projectRoot, blockName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	// Find root domain.yaml
	rootDomain, _, err := FindRootDomain(block.Dir, projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	// Read input
	input := readInput(inputFile)

	// Dispatch based on runtime
	output, err := dispatchBlock(block, rootDomain, projectRoot, input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	writeOutput(output)
}

// handleReason executes a reasoning Block directly from its directory path.
// This is the runner-based path — no discovery needed, the Block directory is given.
// It follows the same stdin/stdout protocol as any other runner (python3, go run, etc.).
func handleReason() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: aglet reason <BlockDir> [input.json]\n")
		os.Exit(1)
	}

	blockDir := os.Args[2]
	inputFile := ""
	if len(os.Args) > 3 {
		inputFile = os.Args[3]
	}

	// Resolve absolute path
	absDir, err := filepath.Abs(blockDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	// Parse the Block's block.yaml directly — no discovery scan needed
	block, err := ParseBlockDir(absDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	// Verify this is actually a reasoning Block
	if block.Config.Runtime != "reasoning" {
		fmt.Fprintf(os.Stderr, "Error: Block '%s' has runtime '%s' — aglet reason only executes reasoning Blocks\n", block.Config.Name, block.Config.Runtime)
		os.Exit(1)
	}

	// Find the project root (needed for tool resolution and provider config)
	projectRoot, err := findProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	// Find root domain.yaml for provider config
	rootDomain, _, err := FindRootDomain(block.Dir, projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	// Read input
	input := readInput(inputFile)

	// Execute through the wrapper for full observability
	output, err := WrapBlock(block, rootDomain, projectRoot, input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	writeOutput(output)
}

// handleServe starts an HTTP dev server that exposes Blocks as endpoints.
func handleServe() {
	// Parse optional --port flag
	port := 3001
	for i := 2; i < len(os.Args)-1; i++ {
		if os.Args[i] == "--port" {
			fmt.Sscanf(os.Args[i+1], "%d", &port)
		}
	}

	projectRoot, err := findProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	rootDomain, _, err := FindRootDomain(projectRoot, projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	if err := StartDevServer(projectRoot, rootDomain, port); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

// handleValidate runs structural validation on the entire project.
// With --deep, generates a judgment-based review checklist for an agent instead.
func handleValidate() {
	deep := false
	jsonOutput := false
	unitFilter := ""

	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--deep":
			deep = true
		case "--json":
			jsonOutput = true
		case "--unit":
			if i+1 < len(os.Args) {
				i++
				unitFilter = os.Args[i]
			}
		}
	}

	projectRoot, err := findProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	if deep {
		RunValidateDeep(projectRoot, jsonOutput, unitFilter)
		return
	}

	if err := RunValidate(projectRoot); err != nil {
		os.Exit(1)
	}
}

// dispatchBlock routes execution through the block wrapper.
// The wrapper handles observability (logging, stderr capture, behavioral memory)
// and delegates to the appropriate executor (process, reasoning) based on runtime.
//
// This function is the universal entry point used by aglet run, aglet pipe,
// and aglet serve. All execution goes through the wrapper.
func dispatchBlock(block *DiscoveredBlock, rootDomain *DomainYaml, projectRoot string, input []byte) ([]byte, error) {
	return WrapBlock(block, rootDomain, projectRoot, input)
}

// readInput reads Block input from a file, stdin, or defaults to empty JSON.
func readInput(inputFile string) []byte {
	if inputFile != "" {
		data, err := os.ReadFile(inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input file: %s\n", err)
			os.Exit(1)
		}
		return data
	}

	// Check if stdin has data
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %s\n", err)
			os.Exit(1)
		}
		return data
	}

	return []byte("{}")
}

// writeOutput writes Block output to stdout with a trailing newline.
func writeOutput(output []byte) {
	os.Stdout.Write(output)
	if len(output) > 0 && output[len(output)-1] != '\n' {
		os.Stdout.WriteString("\n")
	}
}

// findProjectRoot walks up from the current working directory to find the
// nearest directory that contains a root domain.yaml (one without a parent field).
func findProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine working directory: %w", err)
	}

	dir := cwd
	for {
		// Check if this directory contains a root domain.yaml directly
		domainPath := filepath.Join(dir, "domain.yaml")
		if _, err := os.Stat(domainPath); err == nil {
			domain, err := ParseDomainYaml(domainPath)
			if err == nil && domain.Parent == "" {
				return dir, nil
			}
		}

		// Check subdirectories for a root domain.yaml
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				subDomainPath := filepath.Join(dir, entry.Name(), "domain.yaml")
				if _, err := os.Stat(subDomainPath); err == nil {
					domain, err := ParseDomainYaml(subDomainPath)
					if err == nil && domain.Parent == "" {
						return filepath.Join(dir, entry.Name()), nil
					}
				}
			}
		}

		// Walk up
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("no Aglet project found — could not locate a root domain.yaml (without a parent field)")
}
