package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunProcessBlock executes a process Block (runtime: process).
// It resolves the runner, spawns the process, pipes stdin/stdout,
// captures stderr to logs.jsonl, and logs execution metrics.
func RunProcessBlock(block *DiscoveredBlock, rootDomain *DomainYaml, input io.Reader) ([]byte, error) {
	// Check for implementation changes and log if updated
	version := checkAndLogUpdate(block)

	// Resolve implementation file
	implRaw := block.Config.Impl
	if implRaw == "" {
		return nil, fmt.Errorf("Block '%s' has no impl field", block.Config.Name)
	}
	impl := strings.TrimPrefix(implRaw, "./")
	implPath := filepath.Join(block.Dir, impl)

	if _, err := os.Stat(implPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("implementation file not found: %s", implPath)
	}

	// Resolve runner from file extension
	ext := filepath.Ext(impl)
	if ext == "" {
		return nil, fmt.Errorf("implementation file has no extension: %s", impl)
	}

	runner, ok := rootDomain.Runners[ext]
	if !ok {
		return nil, fmt.Errorf("no runner configured for %s files — add to runners section in domain.yaml", ext)
	}

	// Log block start
	logBlockStart(block, version, LogEntry{
		"runner": runner,
		"impl":   impl,
	})
	startTime := time.Now()

	// Build command: split runner into parts and append the impl path
	parts := strings.Fields(runner)
	parts = append(parts, implPath)

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin = input
	cmd.Dir = block.Dir

	// Capture stdout and stderr separately
	stdout, err := cmd.Output()
	durationMs := time.Since(startTime).Milliseconds()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			// Log application stderr
			logApplicationStderr(block, stderr)
			// Log block error
			logBlockError(block, fmt.Sprintf("exited with code %d", exitErr.ExitCode()), LogEntry{
				"exit_code":   exitErr.ExitCode(),
				"duration_ms": durationMs,
			})
			fmt.Fprintf(os.Stderr, "%s", stderr)
			return nil, fmt.Errorf("Block '%s' exited with code %d", block.Config.Name, exitErr.ExitCode())
		}
		logBlockError(block, err.Error(), LogEntry{"duration_ms": durationMs})
		return nil, fmt.Errorf("failed to run Block '%s': %w", block.Config.Name, err)
	}

	// Log successful completion
	logBlockComplete(block, durationMs, len(stdout), nil)

	return stdout, nil
}
