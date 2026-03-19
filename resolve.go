package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ResolvedConfig holds the fully-resolved inheritable config for a block.
// Produced by walking the domain chain: block → nearest domain → parent → root.
type ResolvedConfig struct {
	Runners   map[string]string
	Model     string
	Providers map[string]ProviderConfig
	Stores    map[string]StoreConfig
	Sink      string
	Defaults  DomainDefaults
}

// FindNearestDomainDir walks up from dir looking for a domain.yaml.
// Returns the directory containing the nearest domain.yaml, or "" if none found
// before reaching projectRoot.
func FindNearestDomainDir(dir, projectRoot string) string {
	// Normalize paths for reliable comparison
	dir, _ = filepath.Abs(dir)
	projectRoot, _ = filepath.Abs(projectRoot)

	current := dir
	for {
		candidate := filepath.Join(current, "domain.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return current
		}
		// Stop at project root — don't walk above it
		if current == projectRoot {
			return ""
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "" // filesystem root, give up
		}
		current = parent
	}
}

// ResolveAgletDirForBlock determines the .aglet/{blockName} directory for a block.
// Finds the nearest domain directory and returns {domainDir}/.aglet/{blockName}.
func ResolveAgletDirForBlock(blockDir, blockName, projectRoot string) string {
	domainDir := FindNearestDomainDir(blockDir, projectRoot)
	if domainDir == "" {
		// Fallback: use project root
		domainDir = projectRoot
	}
	return filepath.Join(domainDir, ".aglet", blockName)
}

// ResolveAgletDirForSurface determines the .aglet/{surfaceName} directory for a surface.
func ResolveAgletDirForSurface(surfaceDir, surfaceName, projectRoot string) string {
	domainDir := FindNearestDomainDir(surfaceDir, projectRoot)
	if domainDir == "" {
		domainDir = projectRoot
	}
	return filepath.Join(domainDir, ".aglet", surfaceName)
}

// ResolveSink walks the config chain to determine the sink URL for a block.
// Priority: block.Sink → nearest domain.Aglet.Sink → parent domain → root → "local"
func ResolveSink(block *DiscoveredBlock, projectRoot string) string {
	// Block-level override
	if block.Config.Sink != "" {
		return block.Config.Sink
	}

	// Walk up domains
	dir, _ := filepath.Abs(block.Dir)
	projectRoot, _ = filepath.Abs(projectRoot)

	current := filepath.Dir(dir) // start from block's parent (the domain dir)
	for {
		domainPath := filepath.Join(current, "domain.yaml")
		if _, err := os.Stat(domainPath); err == nil {
			domain, err := ParseDomainYaml(domainPath)
			if err == nil && domain.Aglet.Sink != "" {
				return domain.Aglet.Sink
			}
		}
		if current == projectRoot {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return "local"
}

// ResolveInheritedConfig builds the full resolved config by walking the domain chain.
// block → nearest domain → parent domain → root domain.
// First non-empty value wins for each field.
func ResolveInheritedConfig(block *DiscoveredBlock, projectRoot string) *ResolvedConfig {
	rc := &ResolvedConfig{}

	// Collect all domain configs from nearest to root
	dir, _ := filepath.Abs(block.Dir)
	projectRoot, _ = filepath.Abs(projectRoot)

	current := filepath.Dir(dir)
	for {
		domainPath := filepath.Join(current, "domain.yaml")
		if _, err := os.Stat(domainPath); err == nil {
			domain, err := ParseDomainYaml(domainPath)
			if err == nil {
				// Runners: merge (don't override — nearest takes precedence per-key)
				if rc.Runners == nil && len(domain.Runners) > 0 {
					rc.Runners = domain.Runners
				} else if len(domain.Runners) > 0 {
					// Fill in any missing runners from parent
					for ext, cmd := range domain.Runners {
						if _, exists := rc.Runners[ext]; !exists {
							rc.Runners[ext] = cmd
						}
					}
				}

				// Stores: merge per-key (same pattern as runners — nearest takes precedence)
				if rc.Stores == nil && len(domain.Stores) > 0 {
					rc.Stores = make(map[string]StoreConfig)
					for k, v := range domain.Stores {
						rc.Stores[k] = v
					}
				} else if len(domain.Stores) > 0 {
					for k, v := range domain.Stores {
						if _, exists := rc.Stores[k]; !exists {
							rc.Stores[k] = v
						}
					}
				}

				// Model: first wins
				if rc.Model == "" {
					rc.Model = domain.Defaults.Model
				}

				// Providers: first wins (don't merge individual providers)
				if rc.Providers == nil && len(domain.Providers) > 0 {
					rc.Providers = domain.Providers
				}

				// Sink: first wins
				if rc.Sink == "" {
					rc.Sink = domain.Aglet.Sink
				}

				// Defaults: first wins per field
				if rc.Defaults.Execution == "" {
					rc.Defaults.Execution = domain.Defaults.Execution
				}
				if rc.Defaults.Error == "" {
					rc.Defaults.Error = domain.Defaults.Error
				}
			}
		}

		if current == projectRoot {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	// Block-level overrides
	if block.Config.Model != "" {
		rc.Model = block.Config.Model
	}
	if block.Config.Sink != "" {
		rc.Sink = block.Config.Sink
	}

	// Default sink
	if rc.Sink == "" {
		rc.Sink = "local"
	}

	return rc
}

// envVarPattern matches ${VAR_NAME} references in strings.
var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// resolveEnvVars replaces all ${VAR_NAME} references in s with the
// corresponding environment variable values. Unset variables resolve
// to empty strings. This is used to resolve DSN values at runtime
// so secrets stay out of YAML files.
func resolveEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		return os.Getenv(varName)
	})
}

// ResolveStoreEnvVars builds the AGLET_STORE_{NAME} environment variables
// for a block based on its resolved config. Each store's DSN has ${VAR}
// references resolved against the current environment.
// Returns a slice of "KEY=VALUE" strings ready for cmd.Env.
func ResolveStoreEnvVars(config *ResolvedConfig) []string {
	if len(config.Stores) == 0 {
		return nil
	}
	var envVars []string
	for name, store := range config.Stores {
		envKey := "AGLET_STORE_" + strings.ToUpper(name)
		envVal := resolveEnvVars(store.DSN)
		envVars = append(envVars, envKey+"="+envVal)
	}
	return envVars
}
