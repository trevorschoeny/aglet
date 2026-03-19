package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseBlockDir reads a Block directly from a directory path.
// No scanning — the directory must contain a block.yaml.
func ParseBlockDir(dir string) (*DiscoveredBlock, error) {
	path := filepath.Join(dir, "block.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no block.yaml found in %s: %w", dir, err)
	}

	var config BlockYaml
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("invalid block.yaml in %s: %w", dir, err)
	}

	if config.Runtime == "" {
		config.Runtime = "process"
	}

	return &DiscoveredBlock{Config: config, Dir: dir}, nil
}

// FindBlock scans the project directory for a Block with the given name.
func FindBlock(projectRoot, name string) (*DiscoveredBlock, error) {
	var found *DiscoveredBlock

	err := filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible dirs
		}
		if info.Name() != "block.yaml" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var config BlockYaml
		if err := yaml.Unmarshal(data, &config); err != nil {
			return nil
		}

		if config.Name == name {
			if found != nil {
				return fmt.Errorf("ambiguous Block name '%s': found in both %s and %s", name, found.Dir, filepath.Dir(path))
			}
			found = &DiscoveredBlock{
				Config: config,
				Dir:    filepath.Dir(path),
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	if found == nil {
		return nil, fmt.Errorf("Block '%s' not found in %s", name, projectRoot)
	}

	// Default runtime to "process" if not specified
	if found.Config.Runtime == "" {
		found.Config.Runtime = "process"
	}

	// Load runtime data (.aglet/ path + behavioral memory)
	LoadBlockRuntime(found, projectRoot)

	return found, nil
}

// FindRootDomain walks up from a Block's directory to find the root domain.yaml
// (the one without a parent field).
func FindRootDomain(blockDir, projectRoot string) (*DomainYaml, string, error) {
	dir := blockDir

	for {
		domainPath := filepath.Join(dir, "domain.yaml")
		if _, err := os.Stat(domainPath); err == nil {
			domain, err := ParseDomainYaml(domainPath)
			if err != nil {
				return nil, "", fmt.Errorf("error parsing %s: %w", domainPath, err)
			}
			if domain.Parent == "" {
				return domain, dir, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir || !strings.HasPrefix(parent, projectRoot) {
			break
		}
		dir = parent
	}

	return nil, "", fmt.Errorf("no root domain.yaml found (a domain.yaml without a parent field)")
}

// ParseDomainYaml reads and parses a domain.yaml file.
func ParseDomainYaml(path string) (*DomainYaml, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var domain DomainYaml
	if err := yaml.Unmarshal(data, &domain); err != nil {
		return nil, err
	}
	return &domain, nil
}

// LoadBlockRuntime resolves the .aglet/ directory path and loads behavioral
// memory for a discovered block. This is a post-discovery step — called after
// ParseBlockDir or FindBlock to attach runtime data to the block.
//
// Falls back to reading behavioral_memory from block.yaml for migration
// compatibility with pre-.aglet/ projects.
func LoadBlockRuntime(block *DiscoveredBlock, projectRoot string) {
	// Resolve .aglet/ path
	block.AgletDir = ResolveAgletDirForBlock(block.Dir, block.Config.Name, projectRoot)

	// Try to load memory from .aglet/{blockName}/memory.json
	memPath := filepath.Join(block.AgletDir, "memory.json")
	if data, err := os.ReadFile(memPath); err == nil {
		var mem BehavioralMemory
		if json.Unmarshal(data, &mem) == nil {
			block.BehavioralMemory = &mem
			return
		}
	}

	// No memory.json found — block hasn't been run yet (or pre-migration project)
}

// ResolveModel determines the LLM model for a reasoning Block by checking
// the Block's own config first, then the root domain defaults.
func ResolveModel(block *DiscoveredBlock, rootDomain *DomainYaml) (string, error) {
	if block.Config.Model != "" {
		return block.Config.Model, nil
	}
	if rootDomain.Defaults.Model != "" {
		return rootDomain.Defaults.Model, nil
	}
	return "", fmt.Errorf("no model specified for reasoning Block '%s' and no default model in domain.yaml", block.Config.Name)
}
