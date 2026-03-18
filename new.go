package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// handleNew is the CLI handler for `aglet new <type> <name> [flags]`.
// Creates a new Aglet unit — all required files in one pass, born complete.
func handleNew() {
	if len(os.Args) < 4 {
		printNewUsage()
		os.Exit(1)
	}

	unitType := os.Args[2]
	unitName := os.Args[3]
	flags := parseNewFlags(os.Args[4:])

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot get working directory: %s\n", err)
		os.Exit(1)
	}

	// Units are always created as a subdirectory of cwd named after the unit
	targetDir := filepath.Join(cwd, unitName)

	// Refuse to overwrite existing work
	if _, err := os.Stat(targetDir); err == nil {
		fmt.Fprintf(os.Stderr, "Error: directory '%s' already exists\n", unitName)
		os.Exit(1)
	}

	switch unitType {
	case "block":
		err = newBlock(targetDir, unitName, flags, cwd)
	case "domain":
		err = newDomain(targetDir, unitName, flags, cwd)
	case "surface":
		err = newSurface(targetDir, unitName, flags, cwd)
	case "component":
		err = newComponent(targetDir, unitName, flags, cwd)
	default:
		fmt.Fprintf(os.Stderr, "Unknown type '%s' — must be block, domain, surface, or component\n\n", unitType)
		printNewUsage()
		os.Exit(1)
	}

	if err != nil {
		// Clean up any partially created directory so we don't leave broken state
		os.RemoveAll(targetDir)
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

// --- Unit Creators ---

// newBlock scaffolds a complete Block directory.
// Supports three runtimes: process (default), embedded, reasoning.
// Supports three languages for process/embedded: go (default), ts, py.
// Supports --model for reasoning blocks — mirrors `aglet init --model`.
func newBlock(dir, name string, flags newFlags, cwd string) error {
	runtime := flags.get("runtime", "process")
	lang := flags.get("lang", defaultLangForRuntime(runtime))
	domain := flags.get("domain", inferDomainName(cwd))
	model := flags.get("model", "") // only meaningful for reasoning runtime

	if domain == "" {
		fmt.Fprintf(os.Stderr, "[aglet new] Warning: could not infer domain — set 'domain' in %s/block.yaml manually\n", name)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}

	uuid := generateTypedUUID("b")

	// block.yaml — always required
	if err := writeFile(filepath.Join(dir, "block.yaml"), blockYAML(uuid, name, domain, runtime, lang, model)); err != nil {
		return err
	}

	// intent.md — always required
	if err := writeFile(filepath.Join(dir, "intent.md"), blockIntent(name)); err != nil {
		return err
	}

	// Runtime-specific files
	switch runtime {
	case "reasoning":
		// prompt.md is the implementation for reasoning blocks — no main.* file
		if err := writeFile(filepath.Join(dir, "prompt.md"), reasoningPrompt(name)); err != nil {
			return err
		}
	case "process", "embedded":
		// main.* is the implementation for process and embedded blocks
		implFile, content := blockImpl(name, runtime, lang)
		if err := writeFile(filepath.Join(dir, implFile), content); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown runtime '%s' — must be process, embedded, or reasoning", runtime)
	}

	printCreated("block", name, runtime+"/"+lang, dir)
	return nil
}

// newDomain scaffolds a complete Domain directory.
func newDomain(dir, name string, flags newFlags, cwd string) error {
	// Parent is inferred from the nearest ancestor domain.yaml above cwd
	parent := flags.get("parent", inferDomainName(cwd))

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}

	uuid := generateTypedUUID("d")

	if err := writeFile(filepath.Join(dir, "domain.yaml"), domainYAML(uuid, name, parent)); err != nil {
		return err
	}

	if err := writeFile(filepath.Join(dir, "intent.md"), domainIntent(name)); err != nil {
		return err
	}

	printCreated("domain", name, "", dir)
	return nil
}

// newSurface scaffolds a complete Surface directory.
func newSurface(dir, name string, flags newFlags, cwd string) error {
	domain := flags.get("domain", inferDomainName(cwd))

	if domain == "" {
		fmt.Fprintf(os.Stderr, "[aglet new] Warning: could not infer domain — set 'domain' in %s/surface.yaml manually\n", name)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}

	uuid := generateTypedUUID("s")

	if err := writeFile(filepath.Join(dir, "surface.yaml"), surfaceYAML(uuid, name, domain)); err != nil {
		return err
	}

	if err := writeFile(filepath.Join(dir, "intent.md"), surfaceIntent(name)); err != nil {
		return err
	}

	// Bootstrap entry point
	if err := writeFile(filepath.Join(dir, "main.tsx"), surfaceMain(name)); err != nil {
		return err
	}

	printCreated("surface", name, "", dir)
	return nil
}

// newComponent scaffolds a complete Component directory.
func newComponent(dir, name string, flags newFlags, cwd string) error {
	domain := flags.get("domain", inferDomainName(cwd))

	if domain == "" {
		fmt.Fprintf(os.Stderr, "[aglet new] Warning: could not infer domain — set 'domain' in %s/component.yaml manually\n", name)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}

	uuid := generateTypedUUID("c")

	if err := writeFile(filepath.Join(dir, "component.yaml"), componentYAML(uuid, name, domain)); err != nil {
		return err
	}

	if err := writeFile(filepath.Join(dir, "intent.md"), componentIntent(name)); err != nil {
		return err
	}

	// Implementation file named after the component (convention per spec)
	if err := writeFile(filepath.Join(dir, name+".tsx"), componentImpl(name)); err != nil {
		return err
	}

	printCreated("component", name, "", dir)
	return nil
}

// --- Template Functions ---
// Each returns the file content as a string. Name substitution is done here.

func blockYAML(uuid, name, domain, runtime, lang, model string) string {
	// Determine the impl/prompt/model lines — varies by runtime
	var runtimeLines strings.Builder
	switch runtime {
	case "process", "embedded":
		// impl points to the generated main.* file
		runtimeLines.WriteString(fmt.Sprintf("\nimpl: ./%s\n", mainFile(lang)))
	case "reasoning":
		runtimeLines.WriteString("\nprompt: ./prompt.md\n")
		if model != "" {
			// Explicit model provided — write it directly
			runtimeLines.WriteString(fmt.Sprintf("model: %s\n", model))
		} else {
			// No model provided — include a commented stub so the user knows where to put it.
			// Will use domain default if present; otherwise the block won't run until this is filled.
			runtimeLines.WriteString("# model: claude-sonnet-4-20250514  # inherits from domain default if omitted\n")
		}
	}

	// Role defaults that make sense per runtime
	role := "transformer"
	if runtime == "reasoning" {
		role = "classifier"
	}

	// Observe events — reasoning blocks also track tool.call
	observeEvents := "[start, complete, error]"
	if runtime == "reasoning" {
		observeEvents = "[start, complete, error, tool.call]"
	}

	return fmt.Sprintf(`id: %s
name: %s
description: "TODO: One-line summary"
domain: %s
role: %s

runtime: %s%s
schema:
  in:
    type: object
    properties: {}
    required: []
  out:
    type: object
    properties: {}
    required: []

observe:
  log: ./logs.jsonl
  events: %s
`, uuid, name, domain, role, runtime, runtimeLines.String(), observeEvents)
}

func blockIntent(name string) string {
	return fmt.Sprintf(`# %s

TODO: Describe why this Block exists and what it does.

## Why This Exists

TODO: The architectural and business reason this Block needs to exist as a separate piece.

## Design Decisions

TODO: Key choices and their reasoning. What alternatives were considered and why they were rejected.

## Open Questions

TODO: Unresolved design tensions. Park them here rather than embedding them silently in code.
`, name)
}

func reasoningPrompt(name string) string {
	return fmt.Sprintf(`# %s

TODO: Describe the reasoning framework, constraints, and expected behavior.

## Input

Describe what the input represents and how to interpret it.

## Output

Describe what the output should contain and any constraints on the values.

## Constraints

List any invariants, edge cases, or decision rules the reasoning must follow.
`, name)
}

func blockImpl(name, runtime, lang string) (filename, content string) {
	filename = mainFile(lang)

	if runtime == "embedded" {
		// Embedded blocks export a pure function — no stdin/stdout
		switch lang {
		case "ts":
			content = fmt.Sprintf(`// Input/output types should match block.yaml schema
type %sInput = Record<string, unknown>;
type %sOutput = Record<string, unknown>;

// Exported as a pure function — no state, no side effects
export function %s(input: %sInput): %sOutput {
  // TODO: Implement transformation logic
  return {};
}
`, name, name, name, name, name)
		default:
			content = fmt.Sprintf(`// Input/output types should match block.yaml schema

// Exported as a pure function — no state, no side effects
export function %s(input) {
  // TODO: Implement transformation logic
  return {};
}
`, name)
		}
		return
	}

	// Process block — stdin/stdout protocol
	switch lang {
	case "go":
		content = fmt.Sprintf(`package main

import (
	"encoding/json"
	"os"
)

func main() {
	// In
	var input map[string]interface{}
	json.NewDecoder(os.Stdin).Decode(&input)

	// Transform
	result := %s(input)

	// Out
	json.NewEncoder(os.Stdout).Encode(result)
}

func %s(input map[string]interface{}) map[string]interface{} {
	// TODO: Implement transformation logic
	return map[string]interface{}{}
}
`, name, name)

	case "ts":
		content = fmt.Sprintf(`import { readFileSync } from "fs";

// In
const input = JSON.parse(readFileSync("/dev/stdin", "utf-8"));

// Transform
const result = %s(input);

// Out
console.log(JSON.stringify(result));

function %s(input: Record<string, unknown>): Record<string, unknown> {
  // TODO: Implement transformation logic
  return {};
}
`, name, name)

	case "py":
		// Python uses snake_case convention for function names.
		// Function must be defined before the In/Transform/Out block calls it.
		snakeName := toSnakeCase(name)
		content = fmt.Sprintf(`import json
import sys


def %s(input):
    # TODO: Implement transformation logic
    return {}


# In
input_data = json.load(sys.stdin)

# Transform
result = %s(input_data)

# Out
print(json.dumps(result))
`, snakeName, snakeName)

	default:
		content = fmt.Sprintf("# TODO: Implement %s\n# Runtime: process, language: %s\n", name, lang)
	}

	return
}

func domainYAML(uuid, name, parent string) string {
	parentLine := ""
	if parent != "" {
		parentLine = fmt.Sprintf("\nparent: %s\n", parent)
	}
	return fmt.Sprintf(`id: %s
name: %s%s
# listen: true
# peers:
#   other-domain: "http://localhost:8081"
`, uuid, name, parentLine)
}

func domainIntent(name string) string {
	return fmt.Sprintf(`# %s

TODO: Describe why this domain exists and what it contains.

## Why This Exists

TODO: The reason this grouping exists as a coherent unit within the larger system.
`, name)
}

func surfaceYAML(uuid, name, domain string) string {
	return fmt.Sprintf(`id: %s
name: %s
description: "TODO: One-line summary"
domain: %s
version: 0.1.0

entry: ./main.tsx
framework: react
bundler: vite

dev:
  command: "npm run dev"
  port: 5173

sdk:
  flush_interval: 300

contract: {}
`, uuid, name, domain)
}

func surfaceIntent(name string) string {
	return fmt.Sprintf(`# %s

TODO: Describe the vision for this frontend experience.

## Who This Serves

TODO: The intended audience and their needs.

## Sacred Constraints

TODO: Things that must never be compromised, no matter what.
`, name)
}

func surfaceMain(name string) string {
	return fmt.Sprintf(`import React from "react";
import ReactDOM from "react-dom/client";

// TODO: Import and render your root Component

function App() {
  return <div>TODO: Build %s here.</div>;
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
`, name)
}

func componentYAML(uuid, name, domain string) string {
	return fmt.Sprintf(`id: %s
name: %s
description: "TODO: One-line summary"
domain: %s
role: widget

consumes: []
`, uuid, name, domain)
}

func componentIntent(name string) string {
	return fmt.Sprintf(`# %s

TODO: Describe why this Component exists and what UX it handles.

## Why This Exists

TODO: The UX reason this Component needs to exist as a separate piece.

## Design Decisions

TODO: Key choices and their reasoning.
`, name)
}

func componentImpl(name string) string {
	return fmt.Sprintf(`import { useEffect } from "react";
import { createAglet } from "@aglet/sdk";

interface %sProps {}

export function %s({}: %sProps) {
  // SDK — lifecycle tracking and contract calls
  useEffect(() => {
    const aglet = createAglet("%s");
    aglet.mount();
    return () => {
      aglet.unmount();
      aglet.destroy();
    };
  }, []);

  // TODO: Implement component
  return <div>%s</div>;
}
`, name, name, name, name, name)
}

// --- Helpers ---

// generateTypedUUID creates a UUID v4 with the given prefix (e.g. "b", "s", "c", "d").
// Output format: {prefix}-{8hex}-{4hex}-{4hex}-{4hex}-{12hex}
func generateTypedUUID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is catastrophic — this should never happen
		panic(fmt.Sprintf("crypto/rand failed: %s", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // UUID version 4
	b[8] = (b[8] & 0x3f) | 0x80 // UUID variant (RFC 4122)
	return fmt.Sprintf("%s-%x-%x-%x-%x-%x",
		prefix, b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// inferDomainName walks up from dir looking for the nearest domain.yaml
// and returns its name field. Returns empty string if none found.
func inferDomainName(dir string) string {
	d := dir
	for {
		domainPath := filepath.Join(d, "domain.yaml")
		if _, err := os.Stat(domainPath); err == nil {
			domain, err := ParseDomainYaml(domainPath)
			if err == nil {
				return domain.Name
			}
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return ""
}

// defaultLangForRuntime returns the default implementation language for a runtime.
// Reasoning blocks have no implementation language (prompt.md is the impl).
// Embedded blocks default to TypeScript since they live inside Surfaces.
// Process blocks default to Go.
func defaultLangForRuntime(runtime string) string {
	switch runtime {
	case "embedded":
		return "ts"
	case "reasoning":
		return "" // no lang needed
	default:
		return "go"
	}
}

// mainFile returns the implementation filename for the given language.
func mainFile(lang string) string {
	switch lang {
	case "ts":
		return "main.ts"
	case "py":
		return "main.py"
	default:
		return "main.go"
	}
}

// toSnakeCase converts a PascalCase name to snake_case for Python conventions.
func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(r | 32) // toLower
	}
	return result.String()
}

// writeFile writes content to a file, creating it with standard permissions.
func writeFile(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("could not write %s: %w", filepath.Base(path), err)
	}
	return nil
}

// printCreated reports the created unit and its files, then gives a context-aware
// hint about what to do next. Mirrors the UX of `aglet init`.
func printCreated(unitType, name, variant, dir string) {
	label := unitType
	if variant != "" {
		label = fmt.Sprintf("%s (%s)", unitType, variant)
	}
	fmt.Fprintf(os.Stderr, "[aglet new] Created %s '%s'\n", label, name)

	// List created files
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() {
			fmt.Fprintf(os.Stderr, "  %s/%s\n", name, e.Name())
		}
	}

	// Context-aware next steps — point the user (and any agent) at what matters most
	fmt.Fprintf(os.Stderr, "\nNext:\n")
	switch unitType {
	case "block":
		// Pull runtime out of variant string (e.g. "block (reasoning/)")
		switch {
		case strings.HasPrefix(variant, "reasoning"):
			fmt.Fprintf(os.Stderr, "  Edit intent.md   — why does this reasoning exist?\n")
			fmt.Fprintf(os.Stderr, "  Write prompt.md  — this is your implementation\n")
			fmt.Fprintf(os.Stderr, "  Fill schema      — define in/out in block.yaml\n")
		case strings.HasPrefix(variant, "embedded"):
			fmt.Fprintf(os.Stderr, "  Edit intent.md   — why does this transformation exist?\n")
			fmt.Fprintf(os.Stderr, "  Fill schema      — define in/out in block.yaml\n")
			fmt.Fprintf(os.Stderr, "  Implement        — pure function in main.*\n")
		default: // process
			fmt.Fprintf(os.Stderr, "  Edit intent.md   — why does this block exist?\n")
			fmt.Fprintf(os.Stderr, "  Fill schema      — define in/out in block.yaml\n")
			fmt.Fprintf(os.Stderr, "  Implement        — read stdin, write stdout in main.*\n")
		}
	case "surface":
		fmt.Fprintf(os.Stderr, "  Edit intent.md   — the vision for this frontend\n")
		fmt.Fprintf(os.Stderr, "  Add dependencies — fill the contract in surface.yaml\n")
		fmt.Fprintf(os.Stderr, "  aglet new component <Name>  — add your first Component\n")
	case "component":
		fmt.Fprintf(os.Stderr, "  Edit intent.md   — what UX does this component own?\n")
		fmt.Fprintf(os.Stderr, "  Add consumes     — declare contract dependencies in component.yaml\n")
		fmt.Fprintf(os.Stderr, "  SDK is wired     — mount/unmount are scaffolded in the .tsx\n")
	case "domain":
		fmt.Fprintf(os.Stderr, "  Edit intent.md   — what lives here and why?\n")
		fmt.Fprintf(os.Stderr, "  aglet new block <Name>  — add your first Block\n")
	}
	fmt.Fprintf(os.Stderr, "\n")
}

// newFlags is a simple key→value map for parsed --flag value pairs.
type newFlags map[string]string

// parseNewFlags parses "--key value" pairs from an arg list.
func parseNewFlags(args []string) newFlags {
	flags := newFlags{}
	for i := 0; i+1 < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			key := strings.TrimPrefix(args[i], "--")
			flags[key] = args[i+1]
			i++ // skip the value
		}
	}
	return flags
}

// get returns the flag value or the default if not set.
func (f newFlags) get(key, defaultVal string) string {
	if v, ok := f[key]; ok {
		return v
	}
	return defaultVal
}

// printNewUsage prints help for `aglet new`.
func printNewUsage() {
	fmt.Fprintf(os.Stderr, `Usage: aglet new <type> <name> [flags]

Types:
  block <Name>      Create a new Block (process, embedded, or reasoning)
  domain <Name>     Create a new Domain
  surface <Name>    Create a new Surface
  component <Name>  Create a new Component

Block flags:
  --runtime process|embedded|reasoning   Default: process
  --lang    go|ts|py                     Default: go (process), ts (embedded)
  --domain  <name>                       Default: inferred from nearest domain.yaml
  --model   <model>                      LLM model (reasoning only, e.g. claude-sonnet-4-20250514)

Domain flags:
  --parent  <name>                       Default: inferred from nearest domain.yaml

Surface/Component flags:
  --domain  <name>                       Default: inferred from nearest domain.yaml

Examples:
  aglet new block FetchPage
  aglet new block EmailClassifier --runtime reasoning
  aglet new block EmailClassifier --runtime reasoning --model claude-sonnet-4-20250514
  aglet new block StripSignature --runtime embedded
  aglet new block ParseDate --lang py
  aglet new domain intelligence
  aglet new surface TrevMailClient
  aglet new component ConversationList

`)
}
