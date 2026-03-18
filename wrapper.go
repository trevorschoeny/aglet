package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WrapBlock is the observability shell for block execution.
// It handles: version change detection, logging start/complete/error,
// stderr capture, and behavioral memory updates.
// The actual execution is delegated to the executor (ExecuteProcessBlock,
// ExecuteReasoningBlock), which returns a pure ExecutionResult with no
// side effects on logs.
//
// This is the single entry point for running any block with full observability.
// Every call path — aglet run, aglet pipe, aglet listen, domain listener —
// goes through WrapBlock.
func WrapBlock(block *DiscoveredBlock, rootDomain *DomainYaml, projectRoot string, input []byte) ([]byte, error) {
	// --- Step 1: Detect code changes ---
	// If the implementation file hash has changed since the last logged run,
	// emit a block.updated event. This is an observability concern — the
	// executor doesn't care about versioning.
	version := checkAndLogUpdate(block)

	// --- Step 2: Build pre-execution metadata ---
	// Gather runtime-specific info that the wrapper can include in the
	// block.start log BEFORE execution begins. This is a lightweight probe —
	// no execution happens yet.
	startMeta := buildStartMeta(block, rootDomain)

	// --- Step 3: Log block.start ---
	logBlockStart(block, version, startMeta)
	startTime := time.Now()

	// --- Step 4: Execute via the appropriate executor ---
	// The executor is pure: it runs the block, captures all output (including
	// stderr), and returns everything in an ExecutionResult. It never touches
	// logs.jsonl.
	var result *ExecutionResult

	switch block.Config.Runtime {
	case "process", "":
		result = ExecuteProcessBlock(block, rootDomain, input)
	case "reasoning":
		result = ExecuteReasoningBlock(block, rootDomain, projectRoot, input)
	case "embedded":
		return nil, fmt.Errorf("Block '%s' has runtime 'embedded' — embedded Blocks are internal to Surfaces and cannot be executed externally", block.Config.Name)
	default:
		return nil, fmt.Errorf("Block '%s' has unknown runtime '%s'", block.Config.Name, block.Config.Runtime)
	}

	// --- Step 5: Record duration ---
	durationMs := time.Since(startTime).Milliseconds()

	// --- Step 6: Log stderr (always, not just on error) ---
	// If the implementation wrote anything to stderr, log it as an
	// application-level event. This captures print statements, logging
	// library output, warnings — anything the block wrote to stderr
	// during execution, regardless of exit code.
	if result.Stderr != "" {
		logApplicationStderr(block, result.Stderr)
		// Also print stderr to the CLI's stderr so the developer sees it
		fmt.Fprint(os.Stderr, result.Stderr)
	}

	// --- Step 7: Merge executor metadata with duration for log entries ---
	logMeta := LogEntry{"duration_ms": durationMs}
	for k, v := range result.Meta {
		logMeta[k] = v
	}

	// --- Step 8: Log completion or error ---
	if result.Error != nil {
		logBlockError(block, result.Error.Error(), logMeta)
		return nil, result.Error
	}

	logBlockComplete(block, durationMs, len(result.Output), result.Meta)

	// --- Step 9: Update behavioral memory (best-effort) ---
	// After a successful run, recompute and write behavioral_memory to
	// block.yaml. This is the AML passively observing. Pass nil for
	// allBlocks: the observed_callers cross-scan is too expensive per-run.
	_ = writeBehavioralMemory(block, computeBehavioralMemory(block, nil))

	return result.Output, nil
}

// buildStartMeta gathers lightweight metadata about the block's runtime
// configuration for the block.start log entry. No execution happens here —
// this is just reading config.
func buildStartMeta(block *DiscoveredBlock, rootDomain *DomainYaml) LogEntry {
	meta := LogEntry{}

	switch block.Config.Runtime {
	case "process", "":
		// Include runner and impl info
		if block.Config.Impl != "" {
			impl := strings.TrimPrefix(block.Config.Impl, "./")
			meta["impl"] = impl

			// Resolve runner from file extension
			ext := filepath.Ext(impl)
			if runner, ok := rootDomain.Runners[ext]; ok {
				meta["runner"] = runner
			}
		}

	case "reasoning":
		// Include model and provider info
		if model, err := ResolveModel(block, rootDomain); err == nil {
			meta["model"] = model
		}
		// Resolve provider name for the start log
		if model, err := ResolveModel(block, rootDomain); err == nil {
			if provider, err := ResolveProvider(model, block.Config.Provider, rootDomain.Providers); err == nil {
				meta["provider"] = provider.Name
			}
		}
		if len(block.Config.Tools) > 0 {
			meta["tools"] = block.Config.Tools
		}
	}

	return meta
}
