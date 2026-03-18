package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// WrapBlock is the observability shell for block execution.
// It handles: version change detection, logging start/complete/error,
// stderr capture, behavioral memory updates, and calls forwarding.
// The actual execution is delegated to the executor (ExecuteProcessBlock,
// ExecuteReasoningBlock), which returns a pure ExecutionResult with no
// side effects on logs.
//
// After successful execution, if the block declares `calls` edges, the
// wrapper forwards the output to each downstream block's wrapper. Downstream
// wrappers are pre-warmed (resolved and prepared) concurrently with execution
// so there's no cold-start delay on the handoff.
//
// This is the single entry point for running any block with full observability.
// Every call path — aglet run, aglet pipe, aglet listen, domain listener —
// goes through WrapBlock.
// WrapBlockOptions controls wrapper behavior.
type WrapBlockOptions struct {
	// ForwardCalls controls whether the wrapper forwards output to downstream
	// blocks via the `calls` edges after execution. Defaults to true.
	// Set to false when running blocks in an explicit pipeline (aglet pipe
	// with an EndBlock) to prevent double-execution of downstream blocks.
	ForwardCalls bool
}

// DefaultWrapOptions returns the standard wrapper options: forwarding enabled.
func DefaultWrapOptions() WrapBlockOptions {
	return WrapBlockOptions{ForwardCalls: true}
}

func WrapBlock(block *DiscoveredBlock, rootDomain *DomainYaml, projectRoot string, input []byte) ([]byte, error) {
	return WrapBlockWithOptions(block, rootDomain, projectRoot, input, DefaultWrapOptions())
}

func WrapBlockWithOptions(block *DiscoveredBlock, rootDomain *DomainYaml, projectRoot string, input []byte, opts WrapBlockOptions) ([]byte, error) {
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

	// --- Step 3.5: Pre-warm downstream blocks ---
	// While this block executes, concurrently resolve all blocks declared
	// in `calls`. By the time execution finishes, the downstream wrappers
	// are ready to receive input immediately — no cold-start delay.
	// Only pre-warm if forwarding is enabled (default).
	var preWarmed []*PreWarmedBlock
	if opts.ForwardCalls && len(block.Config.Calls) > 0 {
		preWarmed = preWarmDownstream(block.Config.Calls, projectRoot, rootDomain)
	}

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

	// --- Step 10: Forward output to downstream blocks via `calls` ---
	// If this block declares calls edges, forward the output to each
	// pre-warmed downstream wrapper. The chain propagates automatically —
	// each downstream block's wrapper will in turn forward to ITS calls.
	//
	// For linear pipelines (one call), the final output is the last block's
	// output. For fan-out (multiple calls), all downstream blocks execute
	// concurrently, but we return THIS block's output (fan-out is fire-and-forward).
	if len(preWarmed) > 0 && result.Error == nil {
		forwardOutput := result.Output
		finalOutput, err := forwardToCalls(preWarmed, forwardOutput, rootDomain, projectRoot)
		if err != nil {
			// Log the forwarding error but don't fail this block's execution.
			// The block itself succeeded; the downstream failure is a separate concern.
			fmt.Fprintf(os.Stderr, "[aglet] warning: calls forwarding failed: %s\n", err)
		} else if finalOutput != nil {
			// For linear pipelines, return the final downstream output
			return finalOutput, nil
		}
	}

	return result.Output, nil
}

// PreWarmedBlock holds a resolved block that's ready to receive input.
// Pre-warming happens concurrently with the parent block's execution
// so downstream blocks have zero cold-start delay.
//
// A block can be either local (resolved from the filesystem) or remote
// (routed via the domain's peers table). The Remote flag distinguishes them.
type PreWarmedBlock struct {
	Name     string             // Block name (always set)
	Block    *DiscoveredBlock   // Non-nil for local blocks
	PeerURL  string             // Non-empty for remote blocks (peer domain URL)
	Remote   bool               // True if this block is in a remote domain
	Error    error              // Non-nil if the block couldn't be resolved at all
}

// preWarmDownstream concurrently resolves all blocks in the calls list.
// Each block is discovered locally if possible. If not found locally and
// the domain has peers configured, the block is marked as remote with
// the appropriate peer URL.
func preWarmDownstream(calls []string, projectRoot string, rootDomain *DomainYaml) []*PreWarmedBlock {
	warmed := make([]*PreWarmedBlock, len(calls))
	var wg sync.WaitGroup

	for i, name := range calls {
		wg.Add(1)
		go func(idx int, blockName string) {
			defer wg.Done()

			// Try to resolve locally first
			block, err := FindBlock(projectRoot, blockName)
			if err == nil {
				warmed[idx] = &PreWarmedBlock{Name: blockName, Block: block}
				return
			}

			// Not found locally — check if any peer domain might have it.
			// Block names can be domain-qualified: "payments/PaymentAuth".
			// The domain prefix maps to a peer URL.
			if rootDomain.Peers != nil {
				peerURL := resolvePeerForBlock(blockName, rootDomain.Peers)
				if peerURL != "" {
					warmed[idx] = &PreWarmedBlock{
						Name:    blockName,
						PeerURL: peerURL,
						Remote:  true,
					}
					return
				}
			}

			// Not found locally or in peers
			warmed[idx] = &PreWarmedBlock{
				Name:  blockName,
				Error: fmt.Errorf("block '%s' not found locally or in peers", blockName),
			}
		}(i, name)
	}

	wg.Wait()
	return warmed
}

// resolvePeerForBlock checks if a block name maps to a peer domain.
// Handles domain-qualified names like "payments/PaymentAuth" by extracting
// the domain prefix and looking it up in peers.
// Also does a simple check: if the block name itself matches a peer domain
// name, return that peer (for future use).
func resolvePeerForBlock(blockName string, peers map[string]string) string {
	// Check for domain-qualified name: "payments/PaymentAuth"
	if idx := strings.Index(blockName, "/"); idx > 0 {
		domainPrefix := blockName[:idx]
		if url, ok := peers[domainPrefix]; ok {
			return url
		}
	}

	// No match — the block can't be routed via peers
	return ""
}

// forwardToCalls sends the output to all pre-warmed downstream blocks.
//
// For a single downstream block (linear pipeline), it executes and returns
// the final output — enabling chains to auto-propagate (A → B → C → D).
//
// For multiple downstream blocks (fan-out), all blocks execute concurrently.
// We return nil for the output (the caller uses its own output).
//
// Remote blocks are forwarded via HTTP POST to the peer domain's listener.
// Local blocks are forwarded via WrapBlock directly.
func forwardToCalls(preWarmed []*PreWarmedBlock, output []byte, rootDomain *DomainYaml, projectRoot string) ([]byte, error) {
	// Filter out any blocks that failed to resolve
	var ready []*PreWarmedBlock
	for _, pw := range preWarmed {
		if pw.Error != nil {
			fmt.Fprintf(os.Stderr, "[aglet] warning: could not resolve calls target: %s\n", pw.Error)
			continue
		}
		ready = append(ready, pw)
	}

	if len(ready) == 0 {
		return nil, nil
	}

	// Linear pipeline: single downstream block
	if len(ready) == 1 {
		return executePreWarmed(ready[0], output, rootDomain, projectRoot)
	}

	// Fan-out: multiple downstream blocks — execute concurrently
	var wg sync.WaitGroup
	for _, pw := range ready {
		wg.Add(1)
		go func(target *PreWarmedBlock) {
			defer wg.Done()
			_, err := executePreWarmed(target, output, rootDomain, projectRoot)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[aglet] warning: downstream block '%s' failed: %s\n", target.Name, err)
			}
		}(pw)
	}
	wg.Wait()

	return nil, nil
}

// executePreWarmed runs a pre-warmed block — either locally via WrapBlock
// or remotely via HTTP POST to the peer domain's listener.
func executePreWarmed(pw *PreWarmedBlock, input []byte, rootDomain *DomainYaml, projectRoot string) ([]byte, error) {
	if pw.Remote {
		return callRemoteBlock(pw.PeerURL, pw.Name, input)
	}
	return WrapBlock(pw.Block, rootDomain, projectRoot, input)
}

// callRemoteBlock forwards a block invocation to a remote domain's listener
// via HTTP POST. This is the cross-domain routing mechanism — the wrapper
// sends the output to the peer domain's /block/{name} endpoint.
func callRemoteBlock(peerURL, blockName string, input []byte) ([]byte, error) {
	// Extract the block name without domain prefix for the remote endpoint
	// "payments/PaymentAuth" → "PaymentAuth"
	remoteName := blockName
	if idx := strings.Index(blockName, "/"); idx > 0 {
		remoteName = blockName[idx+1:]
	}

	url := strings.TrimSuffix(peerURL, "/") + "/block/" + remoteName

	resp, err := http.Post(url, "application/json", bytes.NewReader(input))
	if err != nil {
		return nil, fmt.Errorf("remote call to %s failed: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read remote response from %s: %w", url, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote block '%s' at %s returned status %d: %s", blockName, peerURL, resp.StatusCode, string(body))
	}

	return body, nil
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
