package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// --- Types ---

// ValidationError ties an error message to a specific unit.
// If Fix is non-nil, the error can be automatically resolved.
type ValidationError struct {
	Unit    string       // e.g. "FetchPage", "client (surface)", "pipeline (domain)"
	Message string       // human-readable description of the problem
	Fix     func() error // if non-nil, calling this resolves the error
	FixDesc string       // short description of what the fix does (for reporting)
}

// ProjectInventory holds all discovered units for cross-referencing.
type ProjectInventory struct {
	Blocks     []*DiscoveredBlock
	Surfaces   []*DiscoveredSurface
	Components []*DiscoveredComponent
	Domains    []*DiscoveredDomain
	RootDomain *DomainYaml
}

// DiscoveredSurface holds a parsed surface and its location.
type DiscoveredSurface struct {
	Config SurfaceYaml
	Dir    string
}

// DiscoveredDomain holds a parsed domain and its location.
type DiscoveredDomain struct {
	Config DomainYaml
	Dir    string
}

// DiscoveredComponent holds a parsed component and its location.
type DiscoveredComponent struct {
	Config      ComponentYaml
	Dir         string
	SurfaceName string // name of the containing Surface (resolved after discovery)
}

// --- Entry Point ---

// RunValidate discovers all units in the project, runs deterministic checks,
// auto-fixes what it can, and reports remaining errors.
// Returns nil if everything is valid (or was fixed), non-nil if unfixable errors remain.
func RunValidate(projectRoot string) error {
	fmt.Fprintf(os.Stderr, "[aglet validate] Scanning project...\n")

	// Phase 1: Discover all units
	inv, discoveryErrors := discoverProject(projectRoot)

	// Print inventory summary
	fmt.Fprintf(os.Stderr, "[aglet validate] Found %d blocks, %d surfaces, %d components, %d domains\n\n",
		len(inv.Blocks), len(inv.Surfaces), len(inv.Components), len(inv.Domains))

	// Phase 2: Run all checks, accumulating errors
	var allErrors []ValidationError
	allErrors = append(allErrors, discoveryErrors...)
	allErrors = append(allErrors, checkUUIDs(inv)...)
	allErrors = append(allErrors, checkNameFolderMatch(inv)...)
	allErrors = append(allErrors, checkIntentFiles(inv)...)
	allErrors = append(allErrors, checkDomainRefs(inv)...)
	allErrors = append(allErrors, checkBlockFiles(inv)...)
	allErrors = append(allErrors, checkReasoningBlocks(inv)...)
	allErrors = append(allErrors, checkCallsEdges(inv)...)
	allErrors = append(allErrors, checkSchemaCompatibility(inv)...)
	allErrors = append(allErrors, checkCircularDeps(inv)...)
	allErrors = append(allErrors, checkSurfaces(inv)...)
	allErrors = append(allErrors, checkComponents(inv)...)
	allErrors = append(allErrors, checkDomains(inv)...)

	// Phase 3: Apply auto-fixes and separate remaining errors
	var fixed []ValidationError
	var remaining []ValidationError

	for i := range allErrors {
		e := &allErrors[i]
		if e.Fix != nil {
			if err := e.Fix(); err != nil {
				// Fix failed — demote to unfixable error with context
				e.Message = fmt.Sprintf("%s (auto-fix failed: %s)", e.Message, err)
				e.Fix = nil
				remaining = append(remaining, *e)
			} else {
				fixed = append(fixed, *e)
			}
		} else {
			remaining = append(remaining, *e)
		}
	}

	// Phase 4: Print report
	printValidationReport(fixed, remaining, inv)

	if len(remaining) > 0 {
		return fmt.Errorf("%d unfixable error(s) found", len(remaining))
	}
	return nil
}

// --- Discovery ---

// discoverProject walks the project tree once and collects all units.
// Returns the inventory and any errors encountered during parsing.
func discoverProject(projectRoot string) (*ProjectInventory, []ValidationError) {
	inv := &ProjectInventory{}
	var errors []ValidationError

	// Skip these directories during the walk
	skipDirs := map[string]bool{
		"node_modules": true,
		".git":         true,
		".next":        true,
		"dist":         true,
		"build":        true,
		"__pycache__":  true,
		".aglet":       true,
	}

	filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Skip known non-project directories
		if info.IsDir() && skipDirs[info.Name()] {
			return filepath.SkipDir
		}

		// Only process directories
		if !info.IsDir() {
			return nil
		}

		dir := path

		// Check for block.yaml
		if _, err := os.Stat(filepath.Join(dir, "block.yaml")); err == nil {
			block, parseErr := ParseBlockDir(dir)
			if parseErr != nil {
				errors = append(errors, ValidationError{
					Unit:    filepath.Base(dir),
					Message: fmt.Sprintf("failed to parse block.yaml: %s", parseErr),
				})
			} else {
				LoadBlockRuntime(block, projectRoot)
				inv.Blocks = append(inv.Blocks, block)
			}
		}

		// Check for surface.yaml
		if _, err := os.Stat(filepath.Join(dir, "surface.yaml")); err == nil {
			surface, parseErr := parseSurfaceDir(dir)
			if parseErr != nil {
				errors = append(errors, ValidationError{
					Unit:    filepath.Base(dir) + " (surface)",
					Message: fmt.Sprintf("failed to parse surface.yaml: %s", parseErr),
				})
			} else {
				inv.Surfaces = append(inv.Surfaces, surface)
			}
		}

		// Check for component.yaml
		if _, err := os.Stat(filepath.Join(dir, "component.yaml")); err == nil {
			comp, parseErr := parseComponentDir(dir)
			if parseErr != nil {
				errors = append(errors, ValidationError{
					Unit:    filepath.Base(dir) + " (component)",
					Message: fmt.Sprintf("failed to parse component.yaml: %s", parseErr),
				})
			} else {
				inv.Components = append(inv.Components, comp)
			}
		}

		// Check for domain.yaml
		if _, err := os.Stat(filepath.Join(dir, "domain.yaml")); err == nil {
			domain, parseErr := parseDomainDir(dir)
			if parseErr != nil {
				errors = append(errors, ValidationError{
					Unit:    filepath.Base(dir) + " (domain)",
					Message: fmt.Sprintf("failed to parse domain.yaml: %s", parseErr),
				})
			} else {
				inv.Domains = append(inv.Domains, domain)
				// Track root domain (no parent)
				if domain.Config.Parent == "" {
					inv.RootDomain = &domain.Config
				}
			}
		}

		return nil
	})

	// Resolve each Component's parent Surface
	for _, comp := range inv.Components {
		for _, surface := range inv.Surfaces {
			if strings.HasPrefix(comp.Dir, surface.Dir+string(os.PathSeparator)) {
				comp.SurfaceName = surface.Config.Name
				break
			}
		}
	}

	return inv, errors
}

// parseSurfaceDir reads and parses a surface.yaml from a directory.
func parseSurfaceDir(dir string) (*DiscoveredSurface, error) {
	data, err := os.ReadFile(filepath.Join(dir, "surface.yaml"))
	if err != nil {
		return nil, err
	}
	var config SurfaceYaml
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &DiscoveredSurface{Config: config, Dir: dir}, nil
}

// parseComponentDir reads and parses a component.yaml from a directory.
func parseComponentDir(dir string) (*DiscoveredComponent, error) {
	data, err := os.ReadFile(filepath.Join(dir, "component.yaml"))
	if err != nil {
		return nil, err
	}
	var config ComponentYaml
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &DiscoveredComponent{Config: config, Dir: dir}, nil
}

// parseDomainDir reads and parses a domain.yaml from a directory.
func parseDomainDir(dir string) (*DiscoveredDomain, error) {
	domain, err := ParseDomainYaml(filepath.Join(dir, "domain.yaml"))
	if err != nil {
		return nil, err
	}
	return &DiscoveredDomain{Config: *domain, Dir: dir}, nil
}

// --- YAML Fix Helpers ---
// These helpers use map[string]interface{} for round-trip fidelity,
// so unknown fields in the YAML are preserved through the fix.

// updateYAMLFile reads a YAML file as a generic map, applies a mutation, and writes it back.
func updateYAMLFile(path string, mutate func(data map[string]interface{}) error) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return err
	}
	if err := mutate(doc); err != nil {
		return err
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

// createStubFile creates a simple markdown stub file if it doesn't already exist.
func createStubFile(path, title, placeholder string) error {
	content := fmt.Sprintf("# %s\n\n%s\n", title, placeholder)
	return os.WriteFile(path, []byte(content), 0644)
}

// --- Check Functions ---

// checkUUIDs verifies UUID uniqueness and correct prefixes across all units.
func checkUUIDs(inv *ProjectInventory) []ValidationError {
	var errors []ValidationError
	seen := map[string]string{} // ID -> unit label

	// UUID format: prefix + 8-4-4-4-12 hex
	uuidPattern := regexp.MustCompile(`^[a-z]-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

	// Helper to check one unit's ID
	check := func(id, name, expectedPrefix, unitType string) {
		label := name
		if unitType != "" {
			label = name + " (" + unitType + ")"
		}

		if id == "" {
			errors = append(errors, ValidationError{Unit: label, Message: "missing id field"})
			return
		}

		// Check format
		if !uuidPattern.MatchString(id) {
			errors = append(errors, ValidationError{Unit: label, Message: fmt.Sprintf("id '%s' is not a valid typed UUID (expected format: %s + UUID)", id, expectedPrefix)})
			return
		}

		// Check prefix
		if !strings.HasPrefix(id, expectedPrefix) {
			errors = append(errors, ValidationError{Unit: label, Message: fmt.Sprintf("id '%s' has wrong prefix (expected '%s')", id, expectedPrefix)})
		}

		// Check uniqueness
		if existing, ok := seen[id]; ok {
			errors = append(errors, ValidationError{Unit: label, Message: fmt.Sprintf("duplicate id '%s' (also used by %s)", id, existing)})
		} else {
			seen[id] = label
		}
	}

	for _, b := range inv.Blocks {
		check(b.Config.ID, b.Config.Name, "b-", "")
	}
	for _, s := range inv.Surfaces {
		check(s.Config.ID, s.Config.Name, "s-", "surface")
	}
	for _, c := range inv.Components {
		check(c.Config.ID, c.Config.Name, "c-", "component")
	}
	for _, d := range inv.Domains {
		check(d.Config.ID, d.Config.Name, "d-", "domain")
	}

	return errors
}

// checkNameFolderMatch verifies that each unit's name matches its directory name.
// Auto-fix: update the YAML name field to match the folder name.
func checkNameFolderMatch(inv *ProjectInventory) []ValidationError {
	var errors []ValidationError

	// Helper that creates a fixable error for any unit type
	makeNameFix := func(yamlFile, folderName, currentName, label string) ValidationError {
		return ValidationError{
			Unit:    label,
			Message: fmt.Sprintf("name '%s' does not match folder name '%s'", currentName, folderName),
			FixDesc: fmt.Sprintf("name updated to '%s'", folderName),
			Fix: func() error {
				return updateYAMLFile(yamlFile, func(doc map[string]interface{}) error {
					doc["name"] = folderName
					return nil
				})
			},
		}
	}

	for _, b := range inv.Blocks {
		folderName := filepath.Base(b.Dir)
		if b.Config.Name != folderName {
			errors = append(errors, makeNameFix(
				filepath.Join(b.Dir, "block.yaml"), folderName, b.Config.Name, b.Config.Name,
			))
		}
	}
	for _, s := range inv.Surfaces {
		folderName := filepath.Base(s.Dir)
		if s.Config.Name != folderName {
			errors = append(errors, makeNameFix(
				filepath.Join(s.Dir, "surface.yaml"), folderName, s.Config.Name, s.Config.Name+" (surface)",
			))
		}
	}
	for _, c := range inv.Components {
		folderName := filepath.Base(c.Dir)
		if c.Config.Name != folderName {
			errors = append(errors, makeNameFix(
				filepath.Join(c.Dir, "component.yaml"), folderName, c.Config.Name, c.Config.Name+" (component)",
			))
		}
	}
	for _, d := range inv.Domains {
		folderName := filepath.Base(d.Dir)
		if d.Config.Name != folderName {
			errors = append(errors, makeNameFix(
				filepath.Join(d.Dir, "domain.yaml"), folderName, d.Config.Name, d.Config.Name+" (domain)",
			))
		}
	}

	return errors
}

// checkIntentFiles verifies that every unit directory has an intent.md file.
// Auto-fix: create a stub intent.md with a TODO placeholder.
func checkIntentFiles(inv *ProjectInventory) []ValidationError {
	var errors []ValidationError

	checkIntent := func(dir, name, label string) {
		intentPath := filepath.Join(dir, "intent.md")
		if _, err := os.Stat(intentPath); err != nil {
			capturedDir := dir
			capturedName := name
			errors = append(errors, ValidationError{
				Unit:    label,
				Message: "missing intent.md",
				FixDesc: "created stub intent.md",
				Fix: func() error {
					return createStubFile(
						filepath.Join(capturedDir, "intent.md"),
						capturedName,
						"TODO: Describe the intent and purpose of this unit.",
					)
				},
			})
		}
	}

	for _, b := range inv.Blocks {
		checkIntent(b.Dir, b.Config.Name, b.Config.Name)
	}
	for _, s := range inv.Surfaces {
		checkIntent(s.Dir, s.Config.Name, s.Config.Name+" (surface)")
	}
	for _, c := range inv.Components {
		checkIntent(c.Dir, c.Config.Name, c.Config.Name+" (component)")
	}
	for _, d := range inv.Domains {
		checkIntent(d.Dir, d.Config.Name, d.Config.Name+" (domain)")
	}

	return errors
}

// checkDomainRefs verifies that each unit's domain field references an existing domain.
func checkDomainRefs(inv *ProjectInventory) []ValidationError {
	var errors []ValidationError

	// Build a set of known domain names
	domainNames := map[string]bool{}
	for _, d := range inv.Domains {
		domainNames[d.Config.Name] = true
	}

	checkRef := func(domain, label string) {
		if domain == "" {
			return // domain field is optional in some contexts
		}
		if !domainNames[domain] {
			errors = append(errors, ValidationError{
				Unit:    label,
				Message: fmt.Sprintf("domain '%s' does not match any domain.yaml in the project", domain),
			})
		}
	}

	for _, b := range inv.Blocks {
		checkRef(b.Config.Domain, b.Config.Name)
	}
	for _, s := range inv.Surfaces {
		checkRef(s.Config.Domain, s.Config.Name+" (surface)")
	}
	for _, c := range inv.Components {
		checkRef(c.Config.Domain, c.Config.Name+" (component)")
	}

	return errors
}

// checkBlockFiles verifies block-specific file references and schema presence.
func checkBlockFiles(inv *ProjectInventory) []ValidationError {
	var errors []ValidationError

	validRuntimes := map[string]bool{"process": true, "embedded": true, "reasoning": true}

	for _, b := range inv.Blocks {
		name := b.Config.Name

		// Runtime must be valid (ParseBlockDir defaults empty to "process", so this catches bad values)
		if !validRuntimes[b.Config.Runtime] {
			errors = append(errors, ValidationError{
				Unit:    name,
				Message: fmt.Sprintf("invalid runtime '%s' (must be process, embedded, or reasoning)", b.Config.Runtime),
			})
		}

		// Process and embedded blocks need an implementation file
		if b.Config.Runtime == "process" || b.Config.Runtime == "embedded" {
			implPath := b.Config.Impl
			if implPath != "" {
				// Resolve relative to block dir
				fullPath := filepath.Join(b.Dir, implPath)
				if _, err := os.Stat(fullPath); err != nil {
					errors = append(errors, ValidationError{
						Unit:    name,
						Message: fmt.Sprintf("impl '%s' does not exist", implPath),
					})
				}
			} else {
				// No impl declared — check for any main.* file
				found := false
				entries, _ := os.ReadDir(b.Dir)
				for _, e := range entries {
					if strings.HasPrefix(e.Name(), "main.") && !e.IsDir() {
						found = true
						break
					}
				}
				if !found {
					errors = append(errors, ValidationError{
						Unit:    name,
						Message: "no impl field and no main.* file found",
					})
				}
			}
		}

		// Schemas should be present (non-nil)
		if b.Config.Schema.In == nil {
			errors = append(errors, ValidationError{Unit: name, Message: "missing schema.in in block.yaml"})
		}
		if b.Config.Schema.Out == nil {
			errors = append(errors, ValidationError{Unit: name, Message: "missing schema.out in block.yaml"})
		}
	}

	return errors
}

// checkReasoningBlocks runs reasoning-specific checks.
// Auto-fix: create stub prompt.md if missing.
func checkReasoningBlocks(inv *ProjectInventory) []ValidationError {
	var errors []ValidationError

	// Build block lookup for tools validation
	blockByName := map[string]*DiscoveredBlock{}
	for _, b := range inv.Blocks {
		blockByName[b.Config.Name] = b
	}

	for _, b := range inv.Blocks {
		if b.Config.Runtime != "reasoning" {
			continue
		}
		name := b.Config.Name

		// Model must be set or inheritable from root domain
		if b.Config.Model == "" {
			if inv.RootDomain == nil || inv.RootDomain.Defaults.Model == "" {
				errors = append(errors, ValidationError{
					Unit:    name,
					Message: "no model specified and no default model in root domain.yaml",
				})
			}
		}

		// Prompt file must exist — auto-fix: create stub
		promptPath := b.Config.Prompt
		if promptPath == "" {
			promptPath = "./prompt.md" // default
		}
		fullPrompt := filepath.Join(b.Dir, promptPath)
		if _, err := os.Stat(fullPrompt); err != nil {
			capturedPath := fullPrompt
			capturedName := name
			errors = append(errors, ValidationError{
				Unit:    name,
				Message: fmt.Sprintf("prompt file '%s' does not exist", promptPath),
				FixDesc: "created stub prompt.md",
				Fix: func() error {
					return createStubFile(
						capturedPath,
						capturedName+" — Reasoning Prompt",
						"TODO: Define the reasoning framework, constraints, and expected behavior.",
					)
				},
			})
		}

		// Tools must reference existing blocks with runtime process or reasoning
		for _, toolName := range b.Config.Tools {
			toolBlock, ok := blockByName[toolName]
			if !ok {
				errors = append(errors, ValidationError{
					Unit:    name,
					Message: fmt.Sprintf("tools references non-existent block '%s'", toolName),
				})
			} else if toolBlock.Config.Runtime == "embedded" {
				errors = append(errors, ValidationError{
					Unit:    name,
					Message: fmt.Sprintf("tools references embedded block '%s' (only process and reasoning blocks can be tools)", toolName),
				})
			}
		}

		// Reasoning blocks should NOT have a main.* file
		entries, _ := os.ReadDir(b.Dir)
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "main.") && !e.IsDir() {
				errors = append(errors, ValidationError{
					Unit:    name,
					Message: fmt.Sprintf("reasoning block should not have implementation file '%s'", e.Name()),
				})
				break
			}
		}

		// If provider is set, check it exists in root domain providers
		if b.Config.Provider != "" && inv.RootDomain != nil {
			if _, ok := inv.RootDomain.Providers[b.Config.Provider]; !ok {
				errors = append(errors, ValidationError{
					Unit:    name,
					Message: fmt.Sprintf("provider '%s' not found in root domain.yaml providers", b.Config.Provider),
				})
			}
		}
	}

	return errors
}

// checkCallsEdges verifies that every calls reference points to an existing block,
// and checks for divergence between declared tools and observed runtime behavior.
func checkCallsEdges(inv *ProjectInventory) []ValidationError {
	var errors []ValidationError

	// Build block name set
	blockNames := map[string]bool{}
	for _, b := range inv.Blocks {
		blockNames[b.Config.Name] = true
	}

	for _, b := range inv.Blocks {
		// Check calls edges reference existing blocks
		for _, callRef := range b.Config.Calls {
			// Handle domain-qualified names: "domain/BlockName"
			refName := callRef
			if parts := strings.Split(callRef, "/"); len(parts) > 1 {
				refName = parts[len(parts)-1]
			}
			if !blockNames[refName] {
				errors = append(errors, ValidationError{
					Unit:    b.Config.Name,
					Message: fmt.Sprintf("calls references non-existent block '%s'", callRef),
				})
			}
		}

		// Divergence check: compare observed_callees (runtime) against declared tools.
		// observed_callees is built from tool.call log events, which only occur in
		// reasoning blocks. Divergence = signal that design and runtime have drifted.
		// Not auto-fixable — requires a human design decision.
		mem := b.Vitals
		if mem == nil || len(mem.ObservedCallees) == 0 {
			continue
		}

		// Build declared tools set (unqualified names)
		declaredTools := map[string]bool{}
		for _, toolRef := range b.Config.Tools {
			refName := toolRef
			if parts := strings.Split(toolRef, "/"); len(parts) > 1 {
				refName = parts[len(parts)-1]
			}
			declaredTools[refName] = true
		}

		// Undeclared runtime dependency: observed at runtime but not in tools
		for callee := range mem.ObservedCallees {
			if !declaredTools[callee] {
				errors = append(errors, ValidationError{
					Unit:    b.Config.Name,
					Message: fmt.Sprintf("observed calling tool '%s' at runtime but it is not declared in tools", callee),
				})
			}
		}

		// Dead declared tool: in tools but never observed at runtime (after enough data)
		if mem.TotalCalls > 20 {
			for _, toolRef := range b.Config.Tools {
				refName := toolRef
				if parts := strings.Split(toolRef, "/"); len(parts) > 1 {
					refName = parts[len(parts)-1]
				}
				if _, observed := mem.ObservedCallees[refName]; !observed {
					errors = append(errors, ValidationError{
						Unit:    b.Config.Name,
						Message: fmt.Sprintf("declared tool '%s' never observed at runtime after %d calls (may be an untested or dead code path)", toolRef, mem.TotalCalls),
					})
				}
			}
		}
	}

	return errors
}

// checkCircularDeps detects cycles in the calls graph using DFS.
func checkCircularDeps(inv *ProjectInventory) []ValidationError {
	var errors []ValidationError

	// Build adjacency list
	adj := map[string][]string{}
	for _, b := range inv.Blocks {
		adj[b.Config.Name] = b.Config.Calls
	}

	// DFS with coloring: 0=white, 1=gray, 2=black
	color := map[string]int{}
	parent := map[string]string{}

	var dfs func(node string) bool
	dfs = func(node string) bool {
		color[node] = 1 // gray — currently visiting
		for _, neighbor := range adj[node] {
			// Resolve domain-qualified names
			resolvedName := neighbor
			if parts := strings.Split(neighbor, "/"); len(parts) > 1 {
				resolvedName = parts[len(parts)-1]
			}

			if color[resolvedName] == 1 {
				// Found a cycle — trace it back
				cycle := []string{resolvedName, node}
				cur := node
				for cur != resolvedName {
					cur = parent[cur]
					cycle = append(cycle, cur)
				}
				// Reverse for readable order
				for i, j := 0, len(cycle)-1; i < j; i, j = i+1, j-1 {
					cycle[i], cycle[j] = cycle[j], cycle[i]
				}
				errors = append(errors, ValidationError{
					Unit:    node,
					Message: fmt.Sprintf("circular dependency detected: %s", strings.Join(cycle, " -> ")),
				})
				return true
			}
			if color[resolvedName] == 0 {
				parent[resolvedName] = node
				if dfs(resolvedName) {
					return true
				}
			}
		}
		color[node] = 2 // black — done
		return false
	}

	// Visit all nodes
	for _, b := range inv.Blocks {
		if color[b.Config.Name] == 0 {
			dfs(b.Config.Name)
		}
	}

	return errors
}

// checkSurfaces verifies surface-specific constraints.
func checkSurfaces(inv *ProjectInventory) []ValidationError {
	var errors []ValidationError

	// Build block name set for contract validation
	blockNames := map[string]bool{}
	for _, b := range inv.Blocks {
		blockNames[b.Config.Name] = true
	}

	for _, s := range inv.Surfaces {
		label := s.Config.Name + " (surface)"

		// Entry file must exist
		if s.Config.Entry != "" {
			entryPath := filepath.Join(s.Dir, s.Config.Entry)
			if _, err := os.Stat(entryPath); err != nil {
				errors = append(errors, ValidationError{
					Unit:    label,
					Message: fmt.Sprintf("entry file '%s' does not exist", s.Config.Entry),
				})
			}
		}

		// No nested Surfaces — check if any other Surface is a child of this one
		for _, other := range inv.Surfaces {
			if other.Dir == s.Dir {
				continue
			}
			if strings.HasPrefix(other.Dir, s.Dir+string(os.PathSeparator)) {
				errors = append(errors, ValidationError{
					Unit:    label,
					Message: fmt.Sprintf("contains nested surface '%s' — surfaces cannot contain other surfaces", other.Config.Name),
				})
			}
		}

		// Contract dependencies should reference existing blocks or pipelines
		if s.Config.Contract.Dependencies != nil {
			for depName, dep := range s.Config.Contract.Dependencies {
				if dep.Block != "" && !blockNames[dep.Block] {
					errors = append(errors, ValidationError{
						Unit:    label,
						Message: fmt.Sprintf("contract dependency '%s' references non-existent block '%s'", depName, dep.Block),
					})
				}
				// Pipeline references — just check the start block exists
				if dep.Pipeline != "" && !blockNames[dep.Pipeline] {
					errors = append(errors, ValidationError{
						Unit:    label,
						Message: fmt.Sprintf("contract dependency '%s' references non-existent pipeline start block '%s'", depName, dep.Pipeline),
					})
				}
			}
		}
	}

	return errors
}

// checkComponents verifies component-specific constraints.
// Auto-fix: add missing consumes entries (bidirectional sync from contract callers).
func checkComponents(inv *ProjectInventory) []ValidationError {
	var errors []ValidationError

	// Build a map of surface name -> contract dependency names
	surfaceContracts := map[string]map[string]ContractDependency{}
	surfaceDirs := map[string]string{} // surface name -> dir
	for _, s := range inv.Surfaces {
		if s.Config.Contract.Dependencies != nil {
			surfaceContracts[s.Config.Name] = s.Config.Contract.Dependencies
		}
		surfaceDirs[s.Config.Name] = s.Dir
	}

	for _, c := range inv.Components {
		label := c.Config.Name + " (component)"

		// Component must have a parent Surface
		if c.SurfaceName == "" {
			// Not necessarily an error — components can exist in standalone layouts
			// But if they have consumes entries, they need a parent Surface
			if len(c.Config.Consumes) > 0 {
				errors = append(errors, ValidationError{
					Unit:    label,
					Message: "has consumes entries but is not inside a surface directory",
				})
			}
			continue
		}

		// Validate consumes entries exist in the parent Surface's contract
		deps, hasDeps := surfaceContracts[c.SurfaceName]
		for _, consumed := range c.Config.Consumes {
			if !hasDeps {
				errors = append(errors, ValidationError{
					Unit:    label,
					Message: fmt.Sprintf("consumes '%s' but parent surface '%s' has no contract dependencies", consumed, c.SurfaceName),
				})
				continue
			}
			if _, ok := deps[consumed]; !ok {
				errors = append(errors, ValidationError{
					Unit:    label,
					Message: fmt.Sprintf("consumes '%s' which is not in surface '%s' contract", consumed, c.SurfaceName),
				})
			}
		}

		// Bidirectional check: contract callers should list this component,
		// and component consumes should list the contract entry.
		if hasDeps {
			for depName, dep := range deps {
				// Check if this component is listed in the contract callers
				componentInCallers := false
				for _, caller := range dep.Callers {
					// Callers can be domain-qualified: "domain/ComponentName"
					callerName := caller
					if parts := strings.Split(caller, "/"); len(parts) > 1 {
						callerName = parts[len(parts)-1]
					}
					if callerName == c.Config.Name {
						componentInCallers = true
						break
					}
				}

				// Check if component claims to consume this dep
				componentConsumes := false
				for _, consumed := range c.Config.Consumes {
					if consumed == depName {
						componentConsumes = true
						break
					}
				}

				// Bidirectional mismatch: contract lists component as caller but component doesn't consume
				// Auto-fix: add the dep to component's consumes list
				if componentInCallers && !componentConsumes {
					capturedDir := c.Dir
					capturedDep := depName
					errors = append(errors, ValidationError{
						Unit:    label,
						Message: fmt.Sprintf("listed as caller for contract dependency '%s' but does not list it in consumes", depName),
						FixDesc: fmt.Sprintf("added '%s' to consumes", depName),
						Fix: func() error {
							return updateYAMLFile(filepath.Join(capturedDir, "component.yaml"), func(doc map[string]interface{}) error {
								// Get existing consumes or create empty slice
								existing, _ := doc["consumes"].([]interface{})
								doc["consumes"] = append(existing, capturedDep)
								return nil
							})
						},
					})
				}

				// Reverse mismatch: component consumes but contract doesn't list it as caller
				// Auto-fix: add component to contract's callers list
				if componentConsumes && !componentInCallers {
					capturedSurfaceDir := surfaceDirs[c.SurfaceName]
					capturedDepName := depName
					capturedCompName := c.Config.Name
					// Use domain-qualified caller name if the component has a domain
					callerEntry := capturedCompName
					if c.Config.Domain != "" {
						callerEntry = c.Config.Domain + "/" + capturedCompName
					}
					capturedCallerEntry := callerEntry
					errors = append(errors, ValidationError{
						Unit:    label,
						Message: fmt.Sprintf("consumes '%s' but is not listed as a caller in surface contract", depName),
						FixDesc: fmt.Sprintf("added '%s' as caller for '%s' in surface contract", callerEntry, depName),
						Fix: func() error {
							return updateYAMLFile(filepath.Join(capturedSurfaceDir, "surface.yaml"), func(doc map[string]interface{}) error {
								// Navigate to contract.dependencies.{depName}.callers
								contract, _ := doc["contract"].(map[string]interface{})
								if contract == nil {
									return fmt.Errorf("no contract section in surface.yaml")
								}
								dependencies, _ := contract["dependencies"].(map[string]interface{})
								if dependencies == nil {
									return fmt.Errorf("no dependencies in contract")
								}
								dep, _ := dependencies[capturedDepName].(map[string]interface{})
								if dep == nil {
									return fmt.Errorf("dependency '%s' not found in contract", capturedDepName)
								}
								callers, _ := dep["callers"].([]interface{})
								dep["callers"] = append(callers, capturedCallerEntry)
								return nil
							})
						},
					})
				}
			}
		}
	}

	return errors
}

// checkDomains verifies domain-specific constraints.
// Auto-fix: infer parent from filesystem nesting if current parent is invalid.
func checkDomains(inv *ProjectInventory) []ValidationError {
	var errors []ValidationError

	// Build domain name set and dir->name mapping
	domainNames := map[string]bool{}
	domainByDir := map[string]string{} // dir -> domain name
	for _, d := range inv.Domains {
		domainNames[d.Config.Name] = true
		domainByDir[d.Dir] = d.Config.Name
	}

	for _, d := range inv.Domains {
		label := d.Config.Name + " (domain)"

		// Parent must reference an existing domain (if set)
		if d.Config.Parent != "" && !domainNames[d.Config.Parent] {
			// Auto-fix: infer parent from filesystem nesting.
			// Walk up from this domain's directory and find the nearest ancestor
			// that is also a domain directory.
			inferredParent := ""
			dir := filepath.Dir(d.Dir)
			for dir != "." && dir != "/" {
				if name, ok := domainByDir[dir]; ok {
					inferredParent = name
					break
				}
				dir = filepath.Dir(dir)
			}

			if inferredParent != "" {
				capturedDir := d.Dir
				capturedParent := inferredParent
				errors = append(errors, ValidationError{
					Unit:    label,
					Message: fmt.Sprintf("parent '%s' does not match any domain.yaml in the project", d.Config.Parent),
					FixDesc: fmt.Sprintf("parent updated to '%s' (inferred from filesystem)", inferredParent),
					Fix: func() error {
						return updateYAMLFile(filepath.Join(capturedDir, "domain.yaml"), func(doc map[string]interface{}) error {
							doc["parent"] = capturedParent
							return nil
						})
					},
				})
			} else {
				// Can't infer — no ancestor domain found
				errors = append(errors, ValidationError{
					Unit:    label,
					Message: fmt.Sprintf("parent '%s' does not match any domain.yaml in the project (no ancestor domain found to infer from)", d.Config.Parent),
				})
			}
		}
	}

	return errors
}

// --- Report ---

// printValidationReport outputs what was fixed and what remains.
func printValidationReport(fixed, remaining []ValidationError, inv *ProjectInventory) {
	// Print fixes that were applied
	if len(fixed) > 0 {
		for _, f := range fixed {
			fmt.Fprintf(os.Stderr, "  \u2714 Fixed: %s → %s\n", f.Unit, f.FixDesc)
		}
		fmt.Fprintf(os.Stderr, "\n")
	}

	// Print remaining errors grouped by unit
	if len(remaining) > 0 {
		errorsByUnit := map[string][]string{}
		var unitOrder []string
		for _, e := range remaining {
			if _, seen := errorsByUnit[e.Unit]; !seen {
				unitOrder = append(unitOrder, e.Unit)
			}
			errorsByUnit[e.Unit] = append(errorsByUnit[e.Unit], e.Message)
		}
		for _, unit := range unitOrder {
			fmt.Fprintf(os.Stderr, "  %s\n", unit)
			for _, msg := range errorsByUnit[unit] {
				fmt.Fprintf(os.Stderr, "    \u2717 %s\n", msg)
			}
		}
	}

	// Summary
	total := len(fixed) + len(remaining)
	if total == 0 {
		fmt.Fprintf(os.Stderr, "[aglet validate] All checks passed\n")
	} else if len(remaining) == 0 {
		fmt.Fprintf(os.Stderr, "[aglet validate] %d issue(s) found and fixed\n", len(fixed))
	} else if len(fixed) == 0 {
		fmt.Fprintf(os.Stderr, "\n[aglet validate] %d error(s) found\n", len(remaining))
	} else {
		fmt.Fprintf(os.Stderr, "\n[aglet validate] %d issue(s) fixed, %d error(s) remaining\n", len(fixed), len(remaining))
	}
}
