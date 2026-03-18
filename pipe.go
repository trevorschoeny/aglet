package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

// BlockGraph holds all discovered Blocks and their calls edges.
// Used by `aglet pipe` and `aglet serve` to resolve pipelines.
type BlockGraph struct {
	Blocks map[string]*DiscoveredBlock // name → Block
	Edges  map[string][]string         // name → list of downstream Block names
}

// BuildBlockGraph discovers all Blocks in the project and builds
// the calls graph from their block.yaml declarations.
func BuildBlockGraph(projectRoot string) (*BlockGraph, error) {
	allBlocks := discoverAllBlocks(projectRoot)

	graph := &BlockGraph{
		Blocks: make(map[string]*DiscoveredBlock),
		Edges:  make(map[string][]string),
	}

	for _, block := range allBlocks {
		graph.Blocks[block.Config.Name] = block
		graph.Edges[block.Config.Name] = block.Config.Calls
	}

	return graph, nil
}

// FindPath finds a path from start to end in the calls graph using BFS.
// Returns the ordered list of Block names in the pipeline.
func (g *BlockGraph) FindPath(start, end string) ([]string, error) {
	// Validate both endpoints exist
	if _, ok := g.Blocks[start]; !ok {
		return nil, fmt.Errorf("start Block '%s' not found in project", start)
	}
	if _, ok := g.Blocks[end]; !ok {
		return nil, fmt.Errorf("end Block '%s' not found in project", end)
	}

	// If start == end, it's a single Block (not really a pipeline)
	if start == end {
		return []string{start}, nil
	}

	// BFS to find shortest path
	type pathNode struct {
		name string
		path []string
	}

	visited := make(map[string]bool)
	queue := []pathNode{{name: start, path: []string{start}}}
	visited[start] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, next := range g.Edges[current.name] {
			if next == end {
				// Found the path
				return append(current.path, end), nil
			}
			if !visited[next] {
				visited[next] = true
				newPath := make([]string, len(current.path))
				copy(newPath, current.path)
				queue = append(queue, pathNode{
					name: next,
					path: append(newPath, next),
				})
			}
		}
	}

	return nil, fmt.Errorf("no path found from '%s' to '%s' in the calls graph", start, end)
}

// FindPipelineFrom finds the full pipeline starting from a given Block,
// following calls edges until a terminal Block (one with no calls).
// If the graph branches, returns an error — pipelines must be linear.
func (g *BlockGraph) FindPipelineFrom(start string) ([]string, error) {
	if _, ok := g.Blocks[start]; !ok {
		return nil, fmt.Errorf("Block '%s' not found in project", start)
	}

	var path []string
	current := start
	visited := make(map[string]bool)

	for {
		if visited[current] {
			return nil, fmt.Errorf("cycle detected at Block '%s'", current)
		}
		visited[current] = true
		path = append(path, current)

		calls := g.Edges[current]
		if len(calls) == 0 {
			// Terminal Block — pipeline complete
			return path, nil
		}
		if len(calls) > 1 {
			return nil, fmt.Errorf("Block '%s' has multiple calls edges (%s) — pipeline must be linear", current, strings.Join(calls, ", "))
		}
		current = calls[0]
	}
}

// RunPipeline executes a pipeline of Blocks.
//
// If the pipeline has only one block (the start block), it calls WrapBlock
// with forwarding enabled — the calls chain auto-propagates through the
// declared edges. This is the normal case for `aglet pipe StartBlock`.
//
// If the pipeline has multiple blocks (explicit path via start + end),
// it calls each block sequentially with forwarding DISABLED to prevent
// double-execution of downstream blocks.
func RunPipeline(blockNames []string, projectRoot string, rootDomain *DomainYaml, input []byte) ([]byte, error) {
	if len(blockNames) == 0 {
		return nil, fmt.Errorf("empty pipeline")
	}

	// Single block pipeline: just run it with auto-forwarding.
	// The wrapper's calls forwarding handles the rest of the chain.
	if len(blockNames) == 1 {
		block, err := FindBlock(projectRoot, blockNames[0])
		if err != nil {
			return nil, fmt.Errorf("pipeline Block '%s': %w", blockNames[0], err)
		}
		fmt.Fprintf(os.Stderr, "[aglet pipe] %s (auto-forwarding via calls)\n", block.Config.Name)
		return WrapBlock(block, rootDomain, projectRoot, input)
	}

	// Explicit path pipeline: run each block sequentially with forwarding
	// disabled. The explicit path may differ from the calls graph, so we
	// control the execution order manually.
	noForward := WrapBlockOptions{ForwardCalls: false}

	// Resolve all Blocks upfront so we fail fast on missing Blocks
	blocks := make([]*DiscoveredBlock, len(blockNames))
	for i, name := range blockNames {
		block, err := FindBlock(projectRoot, name)
		if err != nil {
			return nil, fmt.Errorf("pipeline Block '%s': %w", name, err)
		}
		blocks[i] = block
	}

	// Execute the pipeline — each Block's output becomes the next Block's input
	current := input
	for i, block := range blocks {
		fmt.Fprintf(os.Stderr, "[aglet pipe] %s (%d/%d)\n", block.Config.Name, i+1, len(blocks))

		output, err := WrapBlockWithOptions(block, rootDomain, projectRoot, current, noForward)
		if err != nil {
			return nil, fmt.Errorf("pipeline failed at Block '%s' (step %d/%d): %w", block.Config.Name, i+1, len(blocks), err)
		}

		// Trim trailing whitespace/newlines for clean piping
		current = bytes.TrimSpace(output)
	}

	return current, nil
}

// handlePipe is the CLI handler for `aglet pipe`.
func handlePipe() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: aglet pipe <StartBlock> [EndBlock]\n")
		fmt.Fprintf(os.Stderr, "\n  If EndBlock is omitted, follows calls edges to the terminal Block.\n")
		fmt.Fprintf(os.Stderr, "  If EndBlock is given, finds the shortest path between start and end.\n\n")
		os.Exit(1)
	}

	startBlock := os.Args[2]
	endBlock := ""
	if len(os.Args) > 3 {
		// Check if arg 3 is an input file or an end block name
		// If it ends in .json, treat it as input file
		if strings.HasSuffix(os.Args[3], ".json") {
			// aglet pipe StartBlock input.json — follow calls to terminal
		} else {
			endBlock = os.Args[3]
		}
	}

	// Find project root
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

	// Build the graph
	graph, err := BuildBlockGraph(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building graph: %s\n", err)
		os.Exit(1)
	}

	// Resolve the pipeline path.
	// If no EndBlock is given, use auto-forwarding: just run the start block
	// and let the wrapper propagate through calls edges automatically.
	// If EndBlock is given, find the explicit path and run it sequentially.
	var pipeline []string
	if endBlock != "" {
		pipeline, err = graph.FindPath(startBlock, endBlock)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "[aglet pipe] Pipeline: %s\n", strings.Join(pipeline, " → "))
	} else {
		// Auto-forwarding mode: just the start block.
		// The wrapper's calls forwarding handles the chain.
		// Still validate the pipeline exists by resolving it, but only run the start.
		fullPipeline, pipeErr := graph.FindPipelineFrom(startBlock)
		if pipeErr != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", pipeErr)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "[aglet pipe] Pipeline: %s (auto-forwarding)\n", strings.Join(fullPipeline, " → "))
		pipeline = []string{startBlock}
	}

	// Read input
	// Check for input file as last arg
	inputFile := ""
	lastArg := os.Args[len(os.Args)-1]
	if strings.HasSuffix(lastArg, ".json") && lastArg != startBlock && lastArg != endBlock {
		inputFile = lastArg
	}
	input := readInput(inputFile)

	// Execute the pipeline
	output, err := RunPipeline(pipeline, projectRoot, rootDomain, input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	writeOutput(output)
}
