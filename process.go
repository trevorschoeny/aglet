package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExecuteProcessBlock is the pure executor for process Blocks (runtime: process).
// It resolves the runner, spawns the subprocess, pipes stdin/stdout, and captures
// ALL stderr — regardless of whether the process succeeds or fails.
//
// This function has NO side effects on logs. It returns everything the wrapper
// needs to write observability data via ExecutionResult.
func ExecuteProcessBlock(block *DiscoveredBlock, rootDomain *DomainYaml, projectRoot string, input []byte) *ExecutionResult {
	// Resolve implementation file
	implRaw := block.Config.Impl
	if implRaw == "" {
		return &ExecutionResult{
			Error: fmt.Errorf("Block '%s' has no impl field", block.Config.Name),
			Meta:  map[string]interface{}{},
		}
	}
	impl := strings.TrimPrefix(implRaw, "./")
	implPath := filepath.Join(block.Dir, impl)

	if _, err := os.Stat(implPath); os.IsNotExist(err) {
		return &ExecutionResult{
			Error: fmt.Errorf("implementation file not found: %s", implPath),
			Meta:  map[string]interface{}{},
		}
	}

	// Resolve runner from file extension
	ext := filepath.Ext(impl)
	if ext == "" {
		return &ExecutionResult{
			Error: fmt.Errorf("implementation file has no extension: %s", impl),
			Meta:  map[string]interface{}{},
		}
	}

	runner, ok := rootDomain.Runners[ext]
	if !ok {
		return &ExecutionResult{
			Error: fmt.Errorf("no runner configured for %s files — add to runners section in domain.yaml", ext),
			Meta:  map[string]interface{}{},
		}
	}

	// Build command: split runner into parts and append the impl path
	parts := strings.Fields(runner)
	parts = append(parts, implPath)

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = block.Dir

	// Inject AGLET_STORE_* environment variables for any stores declared
	// in the domain chain. The block's implementation reads these to connect
	// to databases — no Aglet SDK needed, just standard env vars.
	config := ResolveInheritedConfig(block, projectRoot)
	storeEnvVars := ResolveStoreEnvVars(config)
	if len(storeEnvVars) > 0 {
		// Start with the current environment (so the block inherits PATH, etc.)
		// then append the store variables on top.
		cmd.Env = append(os.Environ(), storeEnvVars...)
	}

	// Capture stdout and stderr into separate buffers.
	// Unlike the old cmd.Output() approach, this captures stderr ALWAYS —
	// not just when the process exits with an error code.
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// Metadata the wrapper will include in log entries
	meta := map[string]interface{}{
		"runner": runner,
		"impl":   impl,
	}

	// Run the subprocess
	err := cmd.Run()
	stderr := stderrBuf.String()

	if err != nil {
		// Extract exit code if available
		if exitErr, ok := err.(*exec.ExitError); ok {
			meta["exit_code"] = exitErr.ExitCode()
			return &ExecutionResult{
				Output: stdoutBuf.Bytes(),
				Stderr: stderr,
				Error:  fmt.Errorf("Block '%s' exited with code %d", block.Config.Name, exitErr.ExitCode()),
				Meta:   meta,
			}
		}
		return &ExecutionResult{
			Stderr: stderr,
			Error:  fmt.Errorf("failed to run Block '%s': %w", block.Config.Name, err),
			Meta:   meta,
		}
	}

	// Success — return output + any stderr the implementation wrote
	return &ExecutionResult{
		Output: stdoutBuf.Bytes(),
		Stderr: stderr,
		Error:  nil,
		Meta:   meta,
	}
}
