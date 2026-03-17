package main

import (
	"fmt"
	"os"
	"strings"
)

// ResolvedProvider holds everything needed to make an API call.
type ResolvedProvider struct {
	Name   string // e.g., "anthropic", "openai", "groq"
	APIKey string
	URL    string // Full base URL for the API
	Format string // "anthropic" or "openai"
}

// builtinProviders defines defaults for well-known providers.
var builtinProviders = map[string]struct {
	URL    string
	Format string
}{
	"anthropic": {URL: "https://api.anthropic.com", Format: "anthropic"},
	"openai":    {URL: "https://api.openai.com", Format: "openai"},
}

// modelPrefixToProvider maps model name prefixes to provider names.
var modelPrefixToProvider = map[string]string{
	"claude":  "anthropic",
	"gpt":     "openai",
	"o1":      "openai",
	"o3":      "openai",
	"o4":      "openai",
}

// ResolveProvider determines the provider for a given model and Block config.
func ResolveProvider(model string, explicitProvider string, providers map[string]ProviderConfig) (*ResolvedProvider, error) {
	// Determine provider name
	providerName := explicitProvider
	if providerName == "" {
		providerName = inferProviderFromModel(model)
	}
	if providerName == "" {
		return nil, fmt.Errorf("cannot infer provider for model '%s' — set 'provider' explicitly in block.yaml", model)
	}

	// Look up provider config
	provConfig, exists := providers[providerName]
	if !exists {
		return nil, fmt.Errorf("provider '%s' not found in domain.yaml providers section", providerName)
	}

	resolved := &ResolvedProvider{Name: providerName}

	// Resolve API key
	if provConfig.Env != "" {
		resolved.APIKey = os.Getenv(provConfig.Env)
		if resolved.APIKey == "" {
			return nil, fmt.Errorf("environment variable %s is not set (required by provider '%s')", provConfig.Env, providerName)
		}
	}

	// Resolve URL and format — use explicit config, fall back to built-in
	if provConfig.URL != "" {
		resolved.URL = provConfig.URL
	} else if builtin, ok := builtinProviders[providerName]; ok {
		resolved.URL = builtin.URL
	} else {
		return nil, fmt.Errorf("provider '%s' has no url configured and is not a built-in provider", providerName)
	}

	if provConfig.Format != "" {
		resolved.Format = provConfig.Format
	} else if builtin, ok := builtinProviders[providerName]; ok {
		resolved.Format = builtin.Format
	} else {
		return nil, fmt.Errorf("provider '%s' has no format configured and is not a built-in provider", providerName)
	}

	return resolved, nil
}

// inferProviderFromModel maps a model name to a provider using known prefixes.
func inferProviderFromModel(model string) string {
	lower := strings.ToLower(model)
	for prefix, provider := range modelPrefixToProvider {
		if strings.HasPrefix(lower, prefix) {
			return provider
		}
	}
	return ""
}
