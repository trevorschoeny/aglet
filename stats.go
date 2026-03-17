package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// handleStats is the CLI handler for `aglet stats [BlockName] [flags]`.
// This is the AML's interface in the CLI — it reads logs.jsonl, distills
// behavioral memory, and optionally writes it back into block.yaml.
//
// The AML's "continuity" in a CLI context isn't a persistent daemon —
// it's the act of distillation itself. Logs are raw experience; stats
// computes the summary. Like sleep is to memory, this is to behavior.
func handleStats() {
	args := os.Args[2:]

	writeBack := false
	jsonOutput := false
	projectWide := false
	blockName := ""
	domainName := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--write":
			writeBack = true
		case "--json":
			jsonOutput = true
		case "--project":
			projectWide = true
		case "--domain":
			if i+1 < len(args) {
				i++
				domainName = args[i]
			}
		default:
			if !strings.HasPrefix(args[i], "--") && blockName == "" {
				blockName = args[i]
			}
		}
	}

	projectRoot, err := findProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	// --domain → domain-level rollup (on-the-fly, not stored)
	if domainName != "" {
		runStatsDomain(projectRoot, domainName, jsonOutput, writeBack)
		return
	}

	// --project (or no block name) → project-wide thermal map
	if projectWide || blockName == "" {
		runStatsProject(projectRoot, jsonOutput, writeBack)
		return
	}

	// Single block view
	block, err := FindBlock(projectRoot, blockName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	// Discover all blocks so observed_callers can be cross-scanned
	inv, _ := discoverProject(projectRoot)
	mem := computeBehavioralMemory(block, inv.Blocks)

	if jsonOutput {
		data, _ := json.MarshalIndent(mem, "", "  ")
		fmt.Println(string(data))
	} else {
		printBlockStats(block.Config.Name, mem)
	}

	if writeBack {
		if err := writeBehavioralMemory(block, mem); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing behavioral_memory: %s\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "[aglet stats] behavioral_memory written to %s/block.yaml\n", block.Config.Name)
	}
}

// blockStatRow pairs a discovered block with its computed behavioral memory.
// Used for sorting and rendering the project-wide thermal map.
type blockStatRow struct {
	block *DiscoveredBlock
	name  string
	mem   *BehavioralMemory
}

// runStatsProject scans all Blocks in the project, computes behavioral
// memory for each, and renders a project-wide thermal map — sorted by
// warmth so the most active Blocks surface to the top.
func runStatsProject(projectRoot string, jsonOutput bool, writeBack bool) {
	inv, _ := discoverProject(projectRoot)

	var results []blockStatRow
	for _, block := range inv.Blocks {
		// Pass all blocks so observed_callers can be cross-scanned
		mem := computeBehavioralMemory(block, inv.Blocks)
		results = append(results, blockStatRow{block, block.Config.Name, mem})
	}

	// Sort by warmth_score descending — hottest blocks first
	sort.Slice(results, func(i, j int) bool {
		return results[i].mem.WarmthScore > results[j].mem.WarmthScore
	})

	if jsonOutput {
		out := map[string]interface{}{}
		for _, r := range results {
			out[r.name] = r.mem
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	} else {
		printProjectStats(results)
	}

	if writeBack {
		for _, r := range results {
			if err := writeBehavioralMemory(r.block, r.mem); err != nil {
				fmt.Fprintf(os.Stderr, "[aglet stats] warning: could not write %s: %s\n", r.name, err)
			}
		}
		fmt.Fprintf(os.Stderr, "[aglet stats] behavioral_memory written to %d block.yaml files\n", len(results))
	}
}

// runStatsDomain computes an on-the-fly rollup for all blocks in a given domain.
// Domain stats are not stored anywhere — they are computed fresh each time.
func runStatsDomain(projectRoot, domainName string, jsonOutput bool, writeBack bool) {
	inv, _ := discoverProject(projectRoot)

	// Filter blocks by domain
	var domainBlocks []*DiscoveredBlock
	for _, block := range inv.Blocks {
		if block.Config.Domain == domainName {
			domainBlocks = append(domainBlocks, block)
		}
	}

	if len(domainBlocks) == 0 {
		fmt.Fprintf(os.Stderr, "No blocks found in domain '%s'\n", domainName)
		os.Exit(1)
	}

	// Compute behavioral memory for each block in the domain.
	// Skip observed_callers cross-scan (pass nil) — domain rollup doesn't need per-block caller graphs.
	type blockResult struct {
		block *DiscoveredBlock
		mem   *BehavioralMemory
	}
	var results []blockResult
	var totalCalls int
	var totalWeightedDurationMs float64
	var totalWeightedError float64
	var totalTokens int
	var totalTokenCalls int
	var totalWarmth float64
	maxWarmth := -1.0
	minWarmth := 2.0
	maxWarmthName := ""
	minWarmthName := ""

	for _, block := range domainBlocks {
		mem := computeBehavioralMemory(block, nil)
		results = append(results, blockResult{block, mem})

		totalCalls += mem.TotalCalls
		totalWeightedDurationMs += mem.AvgRuntimeMs * float64(mem.TotalCalls)
		totalWeightedError += mem.ErrorRate * float64(mem.TotalCalls)
		if mem.TokenAvg > 0 && mem.TotalCalls > 0 {
			totalTokens += mem.TokenAvg * mem.TotalCalls
			totalTokenCalls += mem.TotalCalls
		}
		totalWarmth += mem.WarmthScore
		if mem.WarmthScore > maxWarmth {
			maxWarmth = mem.WarmthScore
			maxWarmthName = block.Config.Name
		}
		if mem.WarmthScore < minWarmth {
			minWarmth = mem.WarmthScore
			minWarmthName = block.Config.Name
		}
	}

	// Derive domain-level aggregates
	avgWarmth := totalWarmth / float64(len(results))

	var avgDurationMs float64
	var avgErrorRate float64
	if totalCalls > 0 {
		avgDurationMs = totalWeightedDurationMs / float64(totalCalls)
		avgErrorRate = totalWeightedError / float64(totalCalls)
	}

	// Domain warmth level: hot if any block is hot, warm if any warm, cold if all cold
	domainWarmth := "cold"
	for _, r := range results {
		if r.mem.WarmthLevel == "hot" {
			domainWarmth = "hot"
			break
		}
		if r.mem.WarmthLevel == "warm" {
			domainWarmth = "warm"
		}
	}

	if jsonOutput {
		out := map[string]interface{}{
			"domain":      domainName,
			"blocks":      len(domainBlocks),
			"warmth":      domainWarmth,
			"avg_warmth":  math.Round(avgWarmth*100) / 100,
			"total_calls": totalCalls,
		}
		if totalCalls > 0 {
			out["avg_runtime_ms"] = math.Round(avgDurationMs*10) / 10
			out["error_rate"] = math.Round(avgErrorRate*10000) / 10000
		}
		if totalTokenCalls > 0 {
			out["total_tokens"] = totalTokens
		}
		if maxWarmthName != "" {
			out["hottest"] = map[string]interface{}{"block": maxWarmthName, "warmth_score": math.Round(maxWarmth*100) / 100}
		}
		if minWarmthName != "" && minWarmthName != maxWarmthName {
			out["coldest"] = map[string]interface{}{"block": minWarmthName, "warmth_score": math.Round(minWarmth*100) / 100}
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("Domain: %s\n", domainName)
		fmt.Println(strings.Repeat("─", 38))
		fmt.Printf("  %-14s %d\n", "Blocks", len(domainBlocks))
		fmt.Printf("  %-14s %s  (%.2f avg)\n", "Warmth", padRight(domainWarmth, 4), avgWarmth)
		fmt.Printf("  %-14s %d\n", "Total calls", totalCalls)
		if totalCalls > 0 {
			fmt.Printf("  %-14s %dms\n", "Avg runtime", int(math.Round(avgDurationMs)))
			fmt.Printf("  %-14s %.1f%%\n", "Error rate", avgErrorRate*100)
		}
		if totalTokenCalls > 0 {
			fmt.Printf("  %-14s ~%s tokens\n", "Token spend", formatTokens(totalTokens))
		}
		if maxWarmthName != "" {
			fmt.Printf("\n  %-14s %s  (%.2f)\n", "Hottest", maxWarmthName, maxWarmth)
		}
		if minWarmthName != "" && minWarmthName != maxWarmthName {
			fmt.Printf("  %-14s %s  (%.2f)\n", "Coldest", minWarmthName, minWarmth)
		}
	}

	if writeBack {
		for _, r := range results {
			if err := writeBehavioralMemory(r.block, r.mem); err != nil {
				fmt.Fprintf(os.Stderr, "[aglet stats] warning: could not write %s: %s\n", r.block.Config.Name, err)
			}
		}
		fmt.Fprintf(os.Stderr, "[aglet stats] behavioral_memory written to %d block.yaml files\n", len(results))
	}
}

// printProjectStats renders the project-wide thermal map.
func printProjectStats(results []blockStatRow) {
	if len(results) == 0 {
		fmt.Println("No blocks found in project.")
		return
	}

	fmt.Printf("%-28s  %-6s  %-6s  %-7s  %-8s  %-8s\n",
		"Block", "Warmth", "Score", "Calls", "Avg ms", "Errors")
	fmt.Println(strings.Repeat("─", 70))

	for _, r := range results {
		m := r.mem

		avgMs := "—"
		if m.TotalCalls > 0 {
			avgMs = fmt.Sprintf("%dms", int(math.Round(m.AvgRuntimeMs)))
		}

		errStr := "—"
		if m.TotalCalls > 0 {
			errStr = fmt.Sprintf("%.1f%%", m.ErrorRate*100)
		}

		fmt.Printf("%-28s  %-6s  %-6.2f  %-7d  %-8s  %-8s\n",
			truncate(r.name, 28), m.WarmthLevel, m.WarmthScore,
			m.TotalCalls, avgMs, errStr)
	}
}

// computeBehavioralMemory reads a Block's logs.jsonl and distills behavioral memory.
//
// Accumulation model: checkpoint + delta. Rather than rereading the entire log
// on every call, it uses the last_updated timestamp in the existing behavioral_memory
// as a checkpoint and processes only new entries. This keeps stats fast even for
// blocks with large log histories.
//
// Reset logic: if a block.updated event (impl file changed) is newer than the
// last checkpoint, the measurement window resets — behavioral memory from the
// old version is discarded. This ensures stats reflect the current code, not
// historical behavior from a different implementation.
//
// allBlocks: if non-nil, an observed_callers cross-scan is performed by reading
// other blocks' logs for tool.call events that reference this block. Pass nil to
// skip this (e.g., the auto-update path in dispatchBlock, where per-run cross-scans
// would be too expensive).
func computeBehavioralMemory(block *DiscoveredBlock, allBlocks []*DiscoveredBlock) *BehavioralMemory {
	logPath := filepath.Join(block.Dir, "logs.jsonl")
	rawData, err := os.ReadFile(logPath)
	if err != nil {
		// No logs yet — block exists but has never been run
		return &BehavioralMemory{
			WarmthLevel: "cold",
			WarmthScore: 0,
			LastUpdated: time.Now().UTC().Format(time.RFC3339),
		}
	}

	lines := strings.Split(strings.TrimSpace(string(rawData)), "\n")

	// --- Step 1: Find the most recent block.updated event (code change = reset point) ---
	var resetTime time.Time
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]interface{}
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		if event, _ := entry["event"].(string); event == "block.updated" {
			if ts, ok := entry["timestamp"].(string); ok {
				if t, parseErr := time.Parse(time.RFC3339, ts); parseErr == nil && t.After(resetTime) {
					resetTime = t
				}
			}
		}
	}

	// --- Step 2: Determine accumulation mode ---
	// If there's existing behavioral_memory and no code change since last stats run → incremental.
	// Otherwise → reset from zero (or from the resetTime forward).
	existing := block.Config.BehavioralMemory
	var windowStart time.Time // zero = process all entries
	doReset := true
	if existing != nil && existing.LastUpdated != "" {
		if lastUpdated, parseErr := time.Parse(time.RFC3339, existing.LastUpdated); parseErr == nil {
			if resetTime.IsZero() || !resetTime.After(lastUpdated) {
				// No new code change since last run → incremental
				doReset = false
				windowStart = lastUpdated
			}
			// else: block.updated is newer than our checkpoint → reset
		}
	}

	// --- Step 3: Seed base counters from existing (incremental mode only) ---
	//
	// For averages, we reconstruct running totals by multiplying existing averages
	// by TotalCalls. This is an approximation (durationCount ≈ TotalCalls), but
	// accurate enough in practice — virtually all block.complete events have duration data.
	var baseCalls int
	var baseDurationMs float64
	var baseDurationCount int
	var baseErrors float64
	var baseTokens int
	var baseTokenCount int
	var lastCalled time.Time
	observedCallees := map[string]int{}
	var versionSince string

	if !doReset && existing != nil {
		baseCalls = existing.TotalCalls
		baseDurationMs = existing.AvgRuntimeMs * float64(existing.TotalCalls)
		baseDurationCount = existing.TotalCalls
		baseErrors = existing.ErrorRate * float64(existing.TotalCalls)
		baseTokens = existing.TokenAvg * existing.TotalCalls
		baseTokenCount = existing.TotalCalls
		if existing.LastCalled != "" {
			if t, parseErr := time.Parse(time.RFC3339, existing.LastCalled); parseErr == nil {
				lastCalled = t
			}
		}
		for k, v := range existing.ObservedCallees {
			observedCallees[k] = v
		}
		versionSince = existing.VersionSince
	}

	// Capture version_since on reset: the timestamp of the triggering block.updated event
	if doReset && !resetTime.IsZero() {
		versionSince = resetTime.UTC().Format(time.RFC3339)
	}

	// --- Step 4: Parse new log entries (only those after windowStart) ---
	var deltaCalls int
	var deltaErrors int
	var deltaDurationMs float64
	var deltaDurationCount int
	var deltaTokens int
	var deltaTokenCount int

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]interface{}
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}

		// Parse entry timestamp and skip entries before the checkpoint
		var entryTime time.Time
		if ts, ok := entry["timestamp"].(string); ok {
			if t, parseErr := time.Parse(time.RFC3339, ts); parseErr == nil {
				entryTime = t
			}
		}
		if !windowStart.IsZero() && !entryTime.After(windowStart) {
			continue
		}

		event, _ := entry["event"].(string)
		switch event {
		case "block.start":
			deltaCalls++
			if entryTime.After(lastCalled) {
				lastCalled = entryTime
			}

		case "block.complete":
			if ms, ok := entry["duration_ms"].(float64); ok {
				deltaDurationMs += ms
				deltaDurationCount++
			}
			inTok, hasIn := entry["input_tokens"].(float64)
			outTok, hasOut := entry["output_tokens"].(float64)
			if hasIn && hasOut {
				deltaTokens += int(inTok + outTok)
				deltaTokenCount++
			}

		case "block.error":
			deltaErrors++

		case "tool.call":
			// Mine which tool blocks this block invoked during reasoning
			if toolName, ok := entry["tool"].(string); ok && toolName != "" {
				observedCallees[toolName]++
			}
		}
	}

	// --- Step 5: Merge base + delta into final counters ---
	totalCalls := baseCalls + deltaCalls
	totalDurationMs := baseDurationMs + deltaDurationMs
	totalDurationCount := baseDurationCount + deltaDurationCount
	totalErrors := baseErrors + float64(deltaErrors)
	totalTokens := baseTokens + deltaTokens
	totalTokenCount := baseTokenCount + deltaTokenCount

	mem := &BehavioralMemory{
		TotalCalls:   totalCalls,
		LastUpdated:  time.Now().UTC().Format(time.RFC3339),
		VersionSince: versionSince,
	}

	if !lastCalled.IsZero() {
		mem.LastCalled = lastCalled.UTC().Format(time.RFC3339)
	}

	if totalDurationCount > 0 {
		mem.AvgRuntimeMs = math.Round((totalDurationMs/float64(totalDurationCount))*10) / 10
	}

	if totalCalls > 0 {
		mem.ErrorRate = math.Round((totalErrors/float64(totalCalls))*10000) / 10000
	}

	if totalTokenCount > 0 {
		avg := totalTokens / totalTokenCount
		if avg > 0 {
			mem.TokenAvg = avg
		}
	}

	if len(observedCallees) > 0 {
		mem.ObservedCallees = observedCallees
	}

	// --- Step 6: Observed callers — cross-block scan ---
	// Only performed when allBlocks is provided (i.e., explicit stats commands).
	// The auto-update path in dispatchBlock passes nil to skip this O(n) scan.
	if allBlocks != nil {
		observedCallers := map[string]int{}
		for _, other := range allBlocks {
			if other.Dir == block.Dir {
				continue
			}
			callerLogData, readErr := os.ReadFile(filepath.Join(other.Dir, "logs.jsonl"))
			if readErr != nil {
				continue
			}
			for _, callerLine := range strings.Split(strings.TrimSpace(string(callerLogData)), "\n") {
				if strings.TrimSpace(callerLine) == "" {
					continue
				}
				var callerEntry map[string]interface{}
				if json.Unmarshal([]byte(callerLine), &callerEntry) != nil {
					continue
				}
				if callerEvent, _ := callerEntry["event"].(string); callerEvent == "tool.call" {
					if toolName, ok := callerEntry["tool"].(string); ok && toolName == block.Config.Name {
						observedCallers[other.Config.Name]++
					}
				}
			}
		}
		if len(observedCallers) > 0 {
			mem.ObservedCallers = observedCallers
		}
	}

	mem.WarmthScore, mem.WarmthLevel = computeWarmth(totalCalls, lastCalled)

	return mem
}

// computeWarmth calculates a warmth score and level from recency and frequency.
//
// Warmth in the CLI context is developer situational awareness — not cache
// readiness (that's for persistent runtimes). A hot block is one being
// actively used and tested. A cold block in the middle of a hot pipeline
// is a signal: have you checked it lately?
//
// Recency is weighted 70%, frequency 30%. Recency dominates because a
// block called once today is more relevant to current work than one
// called 50 times last month.
func computeWarmth(totalCalls int, lastCalled time.Time) (float64, string) {
	var recencyScore float64
	if !lastCalled.IsZero() {
		age := time.Since(lastCalled)
		switch {
		case age < 24*time.Hour:
			recencyScore = 1.0
		case age < 7*24*time.Hour:
			recencyScore = 0.7
		case age < 30*24*time.Hour:
			recencyScore = 0.3
		default:
			recencyScore = 0.0
		}
	}

	// 100+ calls = full frequency score; scales linearly below that
	frequencyScore := math.Min(1.0, float64(totalCalls)/100.0)
	score := math.Round(((recencyScore*0.7)+(frequencyScore*0.3))*100) / 100

	var level string
	switch {
	case score >= 0.7:
		level = "hot"
	case score >= 0.3:
		level = "warm"
	default:
		level = "cold"
	}

	return score, level
}

// printBlockStats renders a human-readable behavioral snapshot for a single Block.
func printBlockStats(name string, mem *BehavioralMemory) {
	fmt.Printf("Block: %s\n", name)
	fmt.Println(strings.Repeat("─", 38))
	fmt.Printf("  %-14s %s  (%.2f)\n", "Warmth", padRight(mem.WarmthLevel, 4), mem.WarmthScore)

	if mem.TotalCalls == 0 {
		fmt.Printf("  %-14s %s\n", "Total calls", "none")
		fmt.Printf("  %-14s %s\n", "Last called", "never")
		return
	}

	fmt.Printf("  %-14s %d\n", "Total calls", mem.TotalCalls)

	if mem.AvgRuntimeMs > 0 {
		fmt.Printf("  %-14s %dms\n", "Avg runtime", int(math.Round(mem.AvgRuntimeMs)))
	}

	fmt.Printf("  %-14s %.1f%%\n", "Error rate", mem.ErrorRate*100)

	if mem.LastCalled != "" {
		if t, err := time.Parse(time.RFC3339, mem.LastCalled); err == nil {
			fmt.Printf("  %-14s %s\n", "Last called", formatRelative(t))
		}
	}

	if mem.VersionSince != "" {
		if t, err := time.Parse(time.RFC3339, mem.VersionSince); err == nil {
			fmt.Printf("  %-14s %s\n", "Version since", t.UTC().Format("2006-01-02"))
		}
	}

	if mem.TokenAvg > 0 {
		fmt.Printf("  %-14s ~%d/call\n", "Token avg", mem.TokenAvg)
	}

	if len(mem.ObservedCallees) > 0 {
		fmt.Printf("  %-14s %s\n", "Calls to", formatToolMap(mem.ObservedCallees))
	}

	if len(mem.ObservedCallers) > 0 {
		fmt.Printf("  %-14s %s\n", "Called by", formatToolMap(mem.ObservedCallers))
	}
}

// formatToolMap formats an observed callers/callees map as "BlockA ×N, BlockB ×M",
// sorted by frequency descending.
func formatToolMap(m map[string]int) string {
	type kv struct {
		k string
		v int
	}
	pairs := make([]kv, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].v != pairs[j].v {
			return pairs[i].v > pairs[j].v
		}
		return pairs[i].k < pairs[j].k
	})
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = fmt.Sprintf("%s ×%d", p.k, p.v)
	}
	return strings.Join(parts, ", ")
}

// writeBehavioralMemory writes the behavioral_memory section into a Block's block.yaml.
// It replaces any existing behavioral_memory section, or appends one if absent.
// All developer-declared fields above the section are preserved exactly.
func writeBehavioralMemory(block *DiscoveredBlock, mem *BehavioralMemory) error {
	blockYamlPath := filepath.Join(block.Dir, "block.yaml")

	existing, err := os.ReadFile(blockYamlPath)
	if err != nil {
		return fmt.Errorf("could not read block.yaml: %w", err)
	}

	// Strip any existing AML section so we can replace it cleanly
	stripped := stripBehavioralMemorySection(string(existing))
	section := buildBehavioralMemoryYAML(mem)

	newContent := strings.TrimRight(stripped, "\n") + "\n\n" + section + "\n"

	return os.WriteFile(blockYamlPath, []byte(newContent), 0644)
}

// stripBehavioralMemorySection removes the AML comment and behavioral_memory
// block from a block.yaml string. Returns everything before that section.
func stripBehavioralMemorySection(content string) string {
	markers := []string{"\n# AML —", "\nbehavioral_memory:"}
	for _, marker := range markers {
		if idx := strings.Index(content, marker); idx != -1 {
			return content[:idx]
		}
	}
	return content
}

// buildBehavioralMemoryYAML produces the YAML string for the behavioral_memory
// section. Uses explicit string building to preserve field order and the comment.
func buildBehavioralMemoryYAML(mem *BehavioralMemory) string {
	var sb strings.Builder
	sb.WriteString("# AML — written by `aglet stats --write`, do not edit manually\n")
	sb.WriteString("behavioral_memory:\n")
	sb.WriteString(fmt.Sprintf("  total_calls: %d\n", mem.TotalCalls))
	sb.WriteString(fmt.Sprintf("  avg_runtime_ms: %.1f\n", mem.AvgRuntimeMs))
	sb.WriteString(fmt.Sprintf("  error_rate: %.4f\n", mem.ErrorRate))
	sb.WriteString(fmt.Sprintf("  warmth_score: %.2f\n", mem.WarmthScore))
	sb.WriteString(fmt.Sprintf("  warmth_level: %s\n", mem.WarmthLevel))
	if mem.LastCalled != "" {
		sb.WriteString(fmt.Sprintf("  last_called: \"%s\"\n", mem.LastCalled))
	}
	if mem.VersionSince != "" {
		sb.WriteString(fmt.Sprintf("  version_since: \"%s\"\n", mem.VersionSince))
	}
	if mem.TokenAvg > 0 {
		sb.WriteString(fmt.Sprintf("  token_avg: %d\n", mem.TokenAvg))
	}
	if len(mem.ObservedCallees) > 0 {
		sb.WriteString("  observed_callees:\n")
		keys := make([]string, 0, len(mem.ObservedCallees))
		for k := range mem.ObservedCallees {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf("    %s: %d\n", k, mem.ObservedCallees[k]))
		}
	}
	if len(mem.ObservedCallers) > 0 {
		sb.WriteString("  observed_callers:\n")
		keys := make([]string, 0, len(mem.ObservedCallers))
		for k := range mem.ObservedCallers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf("    %s: %d\n", k, mem.ObservedCallers[k]))
		}
	}
	sb.WriteString(fmt.Sprintf("  last_updated: \"%s\"\n", mem.LastUpdated))
	return sb.String()
}

// formatTokens formats a token count as a human-readable string (1.2M, 800K, etc.)
func formatTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// formatRelative formats a time as a human-readable relative string.
func formatRelative(t time.Time) string {
	age := time.Since(t)
	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return fmt.Sprintf("%dm ago", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(age.Hours()))
	case age < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(age.Hours()/24))
	default:
		return t.UTC().Format("2006-01-02 15:04 UTC")
	}
}

// truncate shortens a string to max length, appending "…" if needed.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// padRight pads a string to at least n characters with spaces.
func padRight(s string, n int) string {
	for len(s) < n {
		s += " "
	}
	return s
}
