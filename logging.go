package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// LogEntry is a structured log line for logs.jsonl.
// Fields are kept as a map for flexibility — different event types
// carry different fields, and we don't want a rigid struct that
// forces empty fields into every entry.
type LogEntry map[string]interface{}

// GitVersion holds version info for a Block's implementation file,
// resolved from git (with file hash fallback).
type GitVersion struct {
	Commit        string `json:"commit"`                   // Short commit hash of last commit touching this file
	CommitMessage string `json:"commit_message,omitempty"` // First line of that commit's message
	CommitAuthor  string `json:"commit_author,omitempty"`  // Author name
	CommitTime    string `json:"commit_time,omitempty"`    // Author date in ISO 8601
	Dirty         bool   `json:"dirty"`                    // True if file has uncommitted changes
	FileHash      string `json:"file_hash"`                // SHA-256 of the file contents (always present)
}

// resolveGitVersion gets version info for a file from git.
// Falls back to file hash only if git isn't available or the file isn't tracked.
func resolveGitVersion(filePath string) *GitVersion {
	version := &GitVersion{}

	// Always compute the file hash — works even without git
	if data, err := os.ReadFile(filePath); err == nil {
		hash := sha256.Sum256(data)
		version.FileHash = hex.EncodeToString(hash[:8]) // First 8 bytes = 16 hex chars
	}

	dir := filepath.Dir(filePath)

	// Try to get the last commit that touched this file
	// git log -1 --format="%h|%s|%an|%aI" -- <file>
	// Output: short_hash|subject|author_name|author_date_iso
	out, err := exec.Command("git", "-C", dir, "log", "-1",
		"--format=%h|%s|%an|%aI", "--", filePath).Output()
	if err != nil {
		// Git not available or file not tracked — file hash only
		return version
	}

	parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 4)
	if len(parts) == 4 {
		version.Commit = parts[0]
		version.CommitMessage = parts[1]
		version.CommitAuthor = parts[2]
		version.CommitTime = parts[3]
	}

	// Check if the file has uncommitted changes
	// git diff --quiet -- <file> exits non-zero if there are changes
	err = exec.Command("git", "-C", dir, "diff", "--quiet", "--", filePath).Run()
	if err != nil {
		version.Dirty = true
	}
	// Also check staged changes
	err = exec.Command("git", "-C", dir, "diff", "--quiet", "--cached", "--", filePath).Run()
	if err != nil {
		version.Dirty = true
	}

	return version
}

// getLastLoggedHash reads the most recent file_hash from a Block's logs.jsonl.
// Returns empty string if no previous logs exist.
func getLastLoggedHash(block *DiscoveredBlock) string {
	logPath := filepath.Join(block.Dir, "logs.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return ""
	}

	// Walk backwards through lines to find the most recent block.start or block.updated
	// that has a file_hash field
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		var entry map[string]interface{}
		if json.Unmarshal([]byte(lines[i]), &entry) != nil {
			continue
		}
		if hash, ok := entry["file_hash"].(string); ok {
			return hash
		}
	}
	return ""
}

// resolveImplPath returns the absolute path to the Block's implementation file.
// For process Blocks, this is the impl file. For reasoning Blocks, this is prompt.md.
func resolveImplPath(block *DiscoveredBlock) string {
	if block.Config.Runtime == "reasoning" {
		if block.Config.Prompt != "" {
			return filepath.Join(block.Dir, strings.TrimPrefix(block.Config.Prompt, "./"))
		}
		return filepath.Join(block.Dir, "prompt.md")
	}
	if block.Config.Impl != "" {
		return filepath.Join(block.Dir, strings.TrimPrefix(block.Config.Impl, "./"))
	}
	return ""
}

// checkAndLogUpdate checks if a Block's implementation has changed since the
// last logged run. If so, writes a block.updated event. Returns the current
// GitVersion for use in subsequent log entries.
func checkAndLogUpdate(block *DiscoveredBlock) *GitVersion {
	implPath := resolveImplPath(block)
	if implPath == "" {
		return &GitVersion{}
	}

	version := resolveGitVersion(implPath)
	lastHash := getLastLoggedHash(block)

	// If the file hash changed since last logged run, emit block.updated
	if lastHash != "" && lastHash != version.FileHash {
		entry := LogEntry{
			"event":          "block.updated",
			"timestamp":      time.Now().UTC().Format(time.RFC3339),
			"status":         "info",
			"source":         "aglet",
			"block":          block.Config.Name,
			"block_id":       block.Config.ID,
			"file":           filepath.Base(implPath),
			"file_hash":      version.FileHash,
			"prev_hash":      lastHash,
			"commit":         version.Commit,
			"commit_message": version.CommitMessage,
			"commit_author":  version.CommitAuthor,
			"dirty":          version.Dirty,
		}
		appendLog(block, entry)
	}

	return version
}

// logBlockStart writes a block.start event.
func logBlockStart(block *DiscoveredBlock, version *GitVersion, extra LogEntry) {
	entry := LogEntry{
		"event":     "block.start",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"status":    "info",
		"source":    "aglet",
		"block":     block.Config.Name,
		"block_id":  block.Config.ID,
		"runtime":   block.Config.Runtime,
		"file_hash": version.FileHash,
		"commit":    version.Commit,
		"dirty":     version.Dirty,
	}
	// Merge any extra fields (model, provider, etc.)
	for k, v := range extra {
		entry[k] = v
	}
	appendLog(block, entry)
}

// logBlockComplete writes a block.complete event.
func logBlockComplete(block *DiscoveredBlock, durationMs int64, outputBytes int, extra LogEntry) {
	entry := LogEntry{
		"event":        "block.complete",
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"status":       "success",
		"source":       "aglet",
		"block":        block.Config.Name,
		"block_id":     block.Config.ID,
		"duration_ms":  durationMs,
		"output_bytes": outputBytes,
	}
	for k, v := range extra {
		entry[k] = v
	}
	appendLog(block, entry)
}

// logBlockError writes a block.error event.
func logBlockError(block *DiscoveredBlock, errMsg string, extra LogEntry) {
	entry := LogEntry{
		"event":    "block.error",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"status":   "error",
		"source":   "aglet",
		"block":    block.Config.Name,
		"block_id": block.Config.ID,
		"message":  errMsg,
	}
	for k, v := range extra {
		entry[k] = v
	}
	appendLog(block, entry)
}

// logToolCall writes a tool.call event during reasoning.
func logToolCall(block *DiscoveredBlock, toolName string, iteration int) {
	entry := LogEntry{
		"event":     "tool.call",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"status":    "info",
		"source":    "aglet",
		"block":     block.Config.Name,
		"block_id":  block.Config.ID,
		"tool":      toolName,
		"iteration": iteration,
	}
	appendLog(block, entry)
}

// logToolResult writes a tool.result event during reasoning.
func logToolResult(block *DiscoveredBlock, toolName string, durationMs int64, success bool) {
	status := "success"
	if !success {
		status = "error"
	}
	entry := LogEntry{
		"event":       "tool.result",
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"status":      status,
		"source":      "aglet",
		"block":       block.Config.Name,
		"block_id":    block.Config.ID,
		"tool":        toolName,
		"duration_ms": durationMs,
	}
	appendLog(block, entry)
}

// logApplicationStderr writes application-level stderr output.
func logApplicationStderr(block *DiscoveredBlock, stderr string) {
	if strings.TrimSpace(stderr) == "" {
		return
	}
	entry := LogEntry{
		"event":     "stderr",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"status":    "info",
		"source":    "application",
		"block":     block.Config.Name,
		"block_id":  block.Config.ID,
		"message":   strings.TrimSpace(stderr),
	}
	appendLog(block, entry)
}

// logContractCall writes a contract.call event to a surface's logs.jsonl.
// This is called by the block wrapper when it detects surface context in
// the request — the wrapper is the block's network-facing layer, so writing
// to the surface's log is a natural part of its role.
func logContractCall(ctx *SurfaceCallContext, blockName string, durationMs int64, success bool, errMsg string) {
	if ctx == nil || ctx.SurfaceDir == "" {
		return
	}

	status := "success"
	if !success {
		status = "error"
	}

	entry := LogEntry{
		"event":       "contract.call",
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"status":      status,
		"source":      "aglet",
		"surface":     ctx.SurfaceName,
		"contract":    ctx.Contract,
		"block":       blockName,
		"caller":      ctx.Caller,
		"duration_ms": durationMs,
	}
	if errMsg != "" {
		entry["error"] = errMsg
	}

	// Write to the surface's logs.jsonl (not the block's)
	logPath := filepath.Join(ctx.SurfaceDir, "logs.jsonl")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[aglet] warning: could not write surface log to %s: %v\n", logPath, err)
		return
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f.Write(data)
	f.WriteString("\n")
}

// appendLog writes a JSON log entry to the Block's logs.jsonl.
func appendLog(block *DiscoveredBlock, entry LogEntry) {
	logPath := filepath.Join(block.Dir, "logs.jsonl")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		// Best-effort logging — don't let log failures break execution
		fmt.Fprintf(os.Stderr, "[aglet] warning: could not write to %s: %v\n", logPath, err)
		return
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f.Write(data)
	f.WriteString("\n")
}
