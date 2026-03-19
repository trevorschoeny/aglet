package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DeepCheck represents a single judgment-based check for an agent to perform.
// It carries everything the agent needs: which unit, which files to read,
// what question to answer, and any contextual hints.
type DeepCheck struct {
	ID       string   `json:"id"`        // e.g. "intent_accuracy.EmailClassifier"
	Category string   `json:"category"`  // "intent_accuracy", "schema_accuracy", etc.
	Unit     string   `json:"unit"`      // Block/Surface/Component/Domain name
	UnitType string   `json:"unit_type"` // "process_block", "reasoning_block", "embedded_block", "surface", "component", "domain"
	Prompt   string   `json:"prompt"`    // the question/instruction for the agent
	Files    []string `json:"files"`     // relative paths (from project root) to read
	Notes    []string `json:"notes,omitempty"` // contextual hints (stub detected, untested, etc.)
}

// RunValidateDeep generates a judgment-based review checklist for the project.
// Unlike RunValidate (deterministic checks + auto-fix), this produces agent
// instructions — a structured prompt that an AI can act on to perform checks
// that require reading and understanding code.
//
// The CLI does not call any LLM. It generates the checklist; the agent does
// the thinking.
func RunValidateDeep(projectRoot string, jsonOutput bool, unitFilter string) {
	fmt.Fprintf(os.Stderr, "[aglet validate --deep] Scanning project...\n")

	inv, _ := discoverProject(projectRoot)

	checks := generateDeepChecklist(inv, projectRoot, unitFilter)

	if len(checks) == 0 {
		if unitFilter != "" {
			fmt.Fprintf(os.Stderr, "[aglet validate --deep] No checks generated for unit '%s'\n", unitFilter)
		} else {
			fmt.Fprintf(os.Stderr, "[aglet validate --deep] No units found to review\n")
		}
		return
	}

	if jsonOutput {
		printDeepChecklistJSON(checks, inv, projectRoot)
	} else {
		printDeepChecklist(checks, inv, projectRoot, unitFilter)
	}
}

// generateDeepChecklist builds the full list of DeepChecks for the project.
// Each check is self-contained — the agent needs no prior Aglet knowledge to run it.
func generateDeepChecklist(inv *ProjectInventory, projectRoot string, unitFilter string) []DeepCheck {
	var checks []DeepCheck

	// Helper: make a path relative to projectRoot (for readability in output)
	rel := func(absPath string) string {
		r, err := filepath.Rel(projectRoot, absPath)
		if err != nil {
			return absPath
		}
		return r
	}

	// Helper: detect if a file is a stub (contains "TODO:" near the top)
	isStub := func(path string) bool {
		data, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		// Check first 400 bytes for TODO marker
		sample := string(data)
		if len(sample) > 400 {
			sample = sample[:400]
		}
		return strings.Contains(strings.ToUpper(sample), "TODO:")
	}

	// Helper: find the implementation file for a block (by impl field or main.*)
	findImplFile := func(block *DiscoveredBlock) string {
		if block.Config.Impl != "" {
			return filepath.Join(block.Dir, strings.TrimPrefix(block.Config.Impl, "./"))
		}
		for _, ext := range []string{".go", ".ts", ".tsx", ".py", ".js"} {
			p := filepath.Join(block.Dir, "main"+ext)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		return ""
	}

	// Helper: find the implementation file for a component (ComponentName.tsx, etc.)
	findComponentImplFile := func(comp *DiscoveredComponent) string {
		for _, ext := range []string{".tsx", ".ts", ".jsx", ".js", ".go", ".py"} {
			p := filepath.Join(comp.Dir, comp.Config.Name+ext)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		return ""
	}

	// Helper: build warmth notes for a block
	warmthNotes := func(block *DiscoveredBlock) []string {
		mem := block.Vitals
		if mem == nil {
			return []string{"no runtime data — block has never been run; checks are speculative"}
		}
		if mem.TotalCalls == 0 {
			return []string{"0 runs recorded — prompt/logic is untested in practice; scrutinize carefully"}
		}
		if mem.WarmthLevel == "cold" && mem.TotalCalls < 5 {
			return []string{fmt.Sprintf("only %d run(s) recorded — insufficient data to trust runtime behavior", mem.TotalCalls)}
		}
		if mem.WarmthLevel == "hot" && mem.ErrorRate < 0.001 {
			return []string{fmt.Sprintf("battle-tested: %d calls, %.1f%% error rate — implementation is stable", mem.TotalCalls, mem.ErrorRate*100)}
		}
		return nil
	}

	// --- Blocks ---
	for _, b := range inv.Blocks {
		if unitFilter != "" && b.Config.Name != unitFilter {
			continue
		}

		name := b.Config.Name
		runtime := b.Config.Runtime
		if runtime == "" {
			runtime = "process"
		}

		unitType := runtime + "_block"
		intentPath := filepath.Join(b.Dir, "intent.md")
		implFile := findImplFile(b)
		promptFile := filepath.Join(b.Dir, "prompt.md")
		if b.Config.Prompt != "" {
			promptFile = filepath.Join(b.Dir, strings.TrimPrefix(b.Config.Prompt, "./"))
		}
		blockYaml := filepath.Join(b.Dir, "block.yaml")

		// --- Intent Accuracy ---
		intentExists := fileExists(intentPath)
		if intentExists {
			intentStub := isStub(intentPath)
			if intentStub {
				checks = append(checks, DeepCheck{
					ID:       "intent_accuracy." + name,
					Category: "intent_accuracy",
					Unit:     name,
					UnitType: unitType,
					Prompt:   "intent.md is a stub (contains TODO). Write a real intent that explains why this block exists, what design decisions shaped it, and what makes this the right unit boundary.",
					Files:    []string{rel(intentPath)},
					Notes:    []string{"stub detected — requires authoring, not review"},
				})
			} else {
				var implFiles []string
				if runtime == "reasoning" {
					if fileExists(promptFile) {
						implFiles = []string{rel(intentPath), rel(promptFile)}
					} else {
						implFiles = []string{rel(intentPath)}
					}
				} else {
					if implFile != "" {
						implFiles = []string{rel(intentPath), rel(implFile)}
					} else {
						implFiles = []string{rel(intentPath)}
					}
				}

				prompt := ""
				switch runtime {
				case "reasoning":
					prompt = fmt.Sprintf("Does intent.md accurately describe what prompt.md currently implements? Check: (1) do the classification categories / reasoning strategies in the intent match what the prompt instructs? (2) does the prompt implement any logic not mentioned in the intent? (3) does the intent mention capabilities the prompt doesn't have?")
				case "embedded":
					prompt = "Does intent.md accurately describe what the exported function currently does? Check: (1) does the intent match the actual transformation logic? (2) does the code do anything the intent doesn't mention? (3) does the intent describe any behavior the code doesn't implement?"
				default:
					prompt = "Does intent.md accurately describe what the block currently implements? Check: (1) does the stated purpose match the actual transformation? (2) does the code do anything the intent doesn't mention? (3) does the intent describe any behavior the code doesn't implement?"
				}

				notes := warmthNotes(b)
				checks = append(checks, DeepCheck{
					ID:       "intent_accuracy." + name,
					Category: "intent_accuracy",
					Unit:     name,
					UnitType: unitType,
					Prompt:   prompt,
					Files:    implFiles,
					Notes:    notes,
				})
			}
		}

		// --- Schema Accuracy (process and embedded only) ---
		if runtime == "process" || runtime == "embedded" {
			if implFile != "" {
				checks = append(checks, DeepCheck{
					ID:       "schema_accuracy." + name,
					Category: "schema_accuracy",
					Unit:     name,
					UnitType: unitType,
					Prompt:   "Does schema.in in block.yaml list every field the implementation reads from stdin, with correct types and required arrays? Does schema.out list every field it writes to stdout? Flag: missing fields, wrong types, required fields that aren't always produced, optional fields listed as required.",
					Files:    []string{rel(blockYaml), rel(implFile)},
					Notes:    warmthNotes(b),
				})
			}
		}

		// --- Prompt Quality (reasoning only) ---
		if runtime == "reasoning" && fileExists(promptFile) {
			promptStub := isStub(promptFile)
			if promptStub {
				checks = append(checks, DeepCheck{
					ID:       "prompt_quality." + name,
					Category: "prompt_quality",
					Unit:     name,
					UnitType: unitType,
					Prompt:   "prompt.md is a stub (contains TODO). Write a complete system prompt: what the input represents, what judgment to make, constraints to respect, and how to handle edge cases.",
					Files:    []string{rel(promptFile), rel(blockYaml)},
					Notes:    []string{"stub detected — requires authoring, not review"},
				})
			} else {
				notes := warmthNotes(b)
				if b.Vitals == nil || b.Vitals.TotalCalls == 0 {
					notes = append(notes, "prompt has never been run — verify reasoning framework is sound before relying on it")
				}
				checks = append(checks, DeepCheck{
					ID:       "prompt_quality." + name,
					Category: "prompt_quality",
					Unit:     name,
					UnitType: unitType,
					Prompt:   "Is prompt.md comprehensive? Check: (1) does it handle ambiguous or missing input gracefully? (2) are the constraints complete — no obvious edge cases left unaddressed? (3) is the output schema consistent with the categories/values described in the prompt? (4) are there conflicting instructions that could confuse the model?",
					Files:    []string{rel(promptFile), rel(blockYaml)},
					Notes:    notes,
				})
			}
		}

		// --- Single Responsibility ---
		var srNotes []string
		if len(b.Config.Calls) >= 3 {
			srNotes = append(srNotes, fmt.Sprintf("calls %d blocks — verify this is a linear pipeline, not an orchestrator masquerading as a single-responsibility unit", len(b.Config.Calls)))
		}
		if len(b.Config.Tools) >= 4 {
			srNotes = append(srNotes, fmt.Sprintf("declares %d tools — a large tool set can indicate the reasoning is doing multiple distinct jobs", len(b.Config.Tools)))
		}
		// Detect "and" in block name (e.g. ParseAndValidate)
		if strings.Contains(strings.ToLower(name), "and") {
			srNotes = append(srNotes, "block name contains 'and' — potential single-responsibility violation")
		}

		srFiles := []string{rel(blockYaml), rel(intentPath)}
		if implFile != "" {
			srFiles = append(srFiles, rel(implFile))
		} else if runtime == "reasoning" && fileExists(promptFile) {
			srFiles = append(srFiles, rel(promptFile))
		}

		checks = append(checks, DeepCheck{
			ID:       "single_responsibility." + name,
			Category: "single_responsibility",
			Unit:     name,
			UnitType: unitType,
			Prompt:   "Can this block's job be described in one sentence without the word 'and'? If not, identify the split point and suggest two block names that each do one thing.",
			Files:    srFiles,
			Notes:    srNotes,
		})

		// --- Implementation Convention (process and embedded only) ---
		if (runtime == "process" || runtime == "embedded") && implFile != "" {
			implConvPrompt := ""
			if runtime == "embedded" {
				implConvPrompt = "Does the implementation export a pure function named after the block (matching the directory name)? Is it truly stateless — no useState, no useContext, no direct state mutations, no side effects? Does it receive input as a parameter and return output?"
			} else {
				implConvPrompt = "Does the implementation follow the In/Transform/Out convention? Specifically: (1) is there a named function matching the block's directory name that holds the transformation logic? (2) does main() only handle stdin reading, calling that function, and stdout writing? (3) are helper functions below the named block function?"
			}
			checks = append(checks, DeepCheck{
				ID:       "implementation_convention." + name,
				Category: "implementation_convention",
				Unit:     name,
				UnitType: unitType,
				Prompt:   implConvPrompt,
				Files:    []string{rel(implFile)},
			})
		}
	}

	// --- Surfaces ---
	for _, s := range inv.Surfaces {
		if unitFilter != "" && s.Config.Name != unitFilter {
			continue
		}

		name := s.Config.Name
		intentPath := filepath.Join(s.Dir, "intent.md")
		surfaceYaml := filepath.Join(s.Dir, "surface.yaml")

		// Intent accuracy
		if fileExists(intentPath) {
			if isStub(intentPath) {
				checks = append(checks, DeepCheck{
					ID:       "intent_accuracy." + name,
					Category: "intent_accuracy",
					Unit:     name,
					UnitType: "surface",
					Prompt:   "intent.md is a stub. Write a comprehensive founding vision: who uses this surface, what experience it creates, what principles are sacred, and what it deliberately excludes.",
					Files:    []string{rel(intentPath)},
					Notes:    []string{"stub detected — requires authoring, not review"},
				})
			} else {
				checks = append(checks, DeepCheck{
					ID:       "intent_accuracy." + name,
					Category: "intent_accuracy",
					Unit:     name,
					UnitType: "surface",
					Prompt:   "Does intent.md accurately describe the current state of the surface? Check: (1) do the stated principles still guide the actual component structure? (2) does the intent mention the right audience and use cases? (3) is there vision in the intent that hasn't been implemented and should either be removed or flagged as future work?",
					Files:    []string{rel(intentPath), rel(surfaceYaml)},
				})
			}
		}

		// Contract completeness
		checks = append(checks, DeepCheck{
			ID:       "contract_completeness." + name,
			Category: "contract_completeness",
			Unit:     name,
			UnitType: "surface",
			Prompt:   "Is the contract section of surface.yaml complete? Check: (1) does every component that makes an external data call have a corresponding contract entry? (2) does every contract entry have a block/pipeline mapping, callers list, and input/output schemas? (3) are there any dependencies that look like placeholders (empty schemas, missing callers)?",
			Files:    []string{rel(surfaceYaml)},
			Notes:    []string{fmt.Sprintf("surface has %d contract dependencies declared", len(s.Config.Contract.Dependencies))},
		})
	}

	// --- Components ---
	for _, c := range inv.Components {
		if unitFilter != "" && c.Config.Name != unitFilter {
			continue
		}

		name := c.Config.Name
		intentPath := filepath.Join(c.Dir, "intent.md")
		compYaml := filepath.Join(c.Dir, "component.yaml")
		implFile := findComponentImplFile(c)

		// Intent accuracy
		if fileExists(intentPath) && !isStub(intentPath) {
			files := []string{rel(intentPath), rel(compYaml)}
			if implFile != "" {
				files = append(files, rel(implFile))
			}
			checks = append(checks, DeepCheck{
				ID:       "intent_accuracy." + name,
				Category: "intent_accuracy",
				Unit:     name,
				UnitType: "component",
				Prompt:   "Does intent.md accurately describe what this component currently does? Check: (1) does the stated role and responsibility match the implementation? (2) does the intent mention any orchestration logic the component doesn't have? (3) does the component do anything not mentioned in the intent?",
				Files:    files,
			})
		}

		// Logic division (only if there's an implementation to inspect)
		if implFile != "" {
			checks = append(checks, DeepCheck{
				ID:       "logic_division." + name,
				Category: "logic_division",
				Unit:     name,
				UnitType: "component",
				Prompt:   "Does this component contain transformation logic that should be in an embedded block? Transformation logic: data parsing, sorting, filtering, formatting, validation. Orchestration logic (stays in component): event handling, state transitions, deciding when to call things. If transformation logic is found inline, identify it and suggest an embedded block name and location.",
				Files:    []string{rel(implFile)},
			})
		}
	}

	// --- Domains ---
	for _, d := range inv.Domains {
		if unitFilter != "" && d.Config.Name != unitFilter {
			continue
		}

		name := d.Config.Name
		intentPath := filepath.Join(d.Dir, "intent.md")
		domainYaml := filepath.Join(d.Dir, "domain.yaml")

		if fileExists(intentPath) && !isStub(intentPath) {
			checks = append(checks, DeepCheck{
				ID:       "intent_accuracy." + name,
				Category: "intent_accuracy",
				Unit:     name,
				UnitType: "domain",
				Prompt:   "Does intent.md accurately describe the scope and purpose of this domain? Check: (1) do the units inside this domain all belong here, or have some drifted outside the stated scope? (2) are there units that should be here but aren't? (3) are the sacred constraints still respected by the current architecture?",
				Files:    []string{rel(intentPath), rel(domainYaml)},
			})
		}
	}

	return checks
}

// printDeepChecklist renders the deep validation checklist as human-readable markdown.
// The output is designed to be pasted directly into an AI conversation.
func printDeepChecklist(checks []DeepCheck, inv *ProjectInventory, projectRoot string, unitFilter string) {
	// Resolve project name from root domain
	projectName := filepath.Base(projectRoot)
	if inv.RootDomain != nil && inv.RootDomain.Name != "" {
		projectName = inv.RootDomain.Name
	}

	scope := "full project"
	if unitFilter != "" {
		scope = "unit: " + unitFilter
	}

	fmt.Printf("# Aglet Deep Validation — %s\n", projectName)
	fmt.Printf("Generated: %s | %d blocks, %d surfaces, %d components, %d domains | scope: %s\n\n",
		time.Now().UTC().Format("2006-01-02"),
		len(inv.Blocks), len(inv.Surfaces), len(inv.Components), len(inv.Domains),
		scope)
	fmt.Println("Run this checklist against the codebase. For each item, read the listed")
	fmt.Println("files and verify the claim. Report issues as: `[UnitName] — <description>`.")
	fmt.Println("Run `aglet validate` first to clear deterministic errors before this review.")

	// Group checks by category
	categoryOrder := []string{
		"intent_accuracy",
		"schema_accuracy",
		"prompt_quality",
		"single_responsibility",
		"implementation_convention",
		"contract_completeness",
		"logic_division",
	}
	categoryLabels := map[string]string{
		"intent_accuracy":           "Intent Accuracy",
		"schema_accuracy":           "Schema Accuracy",
		"prompt_quality":            "Prompt Quality",
		"single_responsibility":     "Single Responsibility",
		"implementation_convention": "Implementation Convention",
		"contract_completeness":     "Contract Completeness",
		"logic_division":            "Logic Division",
	}
	categoryDescriptions := map[string]string{
		"intent_accuracy":           "Verify intent.md still accurately describes what each unit does. Intent drift is invisible tech debt.",
		"schema_accuracy":           "Verify schema.in/out in block.yaml match what the implementation actually reads and writes.",
		"prompt_quality":            "Verify reasoning prompts are comprehensive and handle edge cases.",
		"single_responsibility":     "Verify each block does exactly one thing, describable without 'and'.",
		"implementation_convention": "Verify process/embedded blocks follow the In/Transform/Out pattern with a named block function.",
		"contract_completeness":     "Verify every external Surface dependency is documented in the contract.",
		"logic_division":            "Verify transformation logic lives in embedded blocks, not inline in components.",
	}

	// Index checks by category
	byCategory := map[string][]DeepCheck{}
	for _, c := range checks {
		byCategory[c.Category] = append(byCategory[c.Category], c)
	}

	for _, cat := range categoryOrder {
		catChecks, ok := byCategory[cat]
		if !ok {
			continue
		}

		fmt.Printf("\n---\n\n## %s\n\n", categoryLabels[cat])
		if desc, ok := categoryDescriptions[cat]; ok {
			fmt.Printf("_%s_\n\n", desc)
		}

		for _, check := range catChecks {
			unitTypeLabel := formatUnitTypeLabel(check.UnitType)
			fmt.Printf("- [ ] **%s** (%s)\n", check.Unit, unitTypeLabel)
			// Word-wrap the prompt at ~80 chars with indentation
			fmt.Printf("  %s\n", check.Prompt)
			if len(check.Notes) > 0 {
				for _, note := range check.Notes {
					fmt.Printf("  ⚠ %s\n", note)
				}
			}
			if len(check.Files) > 0 {
				fileParts := make([]string, len(check.Files))
				for i, f := range check.Files {
					fileParts[i] = "`" + f + "`"
				}
				fmt.Printf("  Files: %s\n", strings.Join(fileParts, ", "))
			}
			fmt.Println()
		}
	}

	fmt.Printf("---\n\n%d checks across %d categories\n", len(checks), len(byCategory))
}

// printDeepChecklistJSON renders the deep validation checklist as JSON.
// Designed for programmatic agent consumption.
func printDeepChecklistJSON(checks []DeepCheck, inv *ProjectInventory, projectRoot string) {
	projectName := filepath.Base(projectRoot)
	if inv.RootDomain != nil && inv.RootDomain.Name != "" {
		projectName = inv.RootDomain.Name
	}

	out := map[string]interface{}{
		"project":      projectName,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"summary": map[string]int{
			"blocks":     len(inv.Blocks),
			"surfaces":   len(inv.Surfaces),
			"components": len(inv.Components),
			"domains":    len(inv.Domains),
		},
		"preamble": "For each check: read the listed files and verify the claim. Report issues as the unit name and a description of what's wrong. Run `aglet validate` first to clear deterministic errors.",
		"checks":   checks,
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(data))
}

// formatUnitTypeLabel converts an internal unit_type string to a human-readable label.
func formatUnitTypeLabel(unitType string) string {
	switch unitType {
	case "process_block":
		return "process block"
	case "reasoning_block":
		return "reasoning block"
	case "embedded_block":
		return "embedded block"
	case "surface":
		return "surface"
	case "component":
		return "component"
	case "domain":
		return "domain"
	default:
		return unitType
	}
}

// fileExists returns true if the file exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
