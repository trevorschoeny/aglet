package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// handleInit is the CLI handler for `aglet init <ProjectName> [flags]`.
// Bootstraps a new Aglet project — creates the root domain directory
// with a domain.yaml and intent.md, ready to scaffold Blocks and Surfaces into.
func handleInit() {
	if len(os.Args) < 3 {
		printInitUsage()
		os.Exit(1)
	}

	projectName := os.Args[2]
	flags := parseNewFlags(os.Args[3:])
	model := flags.get("model", "")

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot get working directory: %s\n", err)
		os.Exit(1)
	}

	projectDir := filepath.Join(cwd, projectName)

	// Refuse to overwrite an existing directory
	if _, err := os.Stat(projectDir); err == nil {
		fmt.Fprintf(os.Stderr, "Error: directory '%s' already exists\n", projectName)
		os.Exit(1)
	}

	if err := os.MkdirAll(projectDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not create directory: %s\n", err)
		os.Exit(1)
	}

	// Write root domain.yaml
	domainPath := filepath.Join(projectDir, "domain.yaml")
	if err := os.WriteFile(domainPath, []byte(rootDomainYAML(projectName, model)), 0644); err != nil {
		os.RemoveAll(projectDir)
		fmt.Fprintf(os.Stderr, "Error: could not write domain.yaml: %s\n", err)
		os.Exit(1)
	}

	// Write root intent.md
	intentPath := filepath.Join(projectDir, "intent.md")
	if err := os.WriteFile(intentPath, []byte(rootDomainIntent(projectName)), 0644); err != nil {
		os.RemoveAll(projectDir)
		fmt.Fprintf(os.Stderr, "Error: could not write intent.md: %s\n", err)
		os.Exit(1)
	}

	// Report what was created
	fmt.Fprintf(os.Stderr, "[aglet init] Created project '%s'\n", projectName)
	fmt.Fprintf(os.Stderr, "  %s/domain.yaml\n", projectName)
	fmt.Fprintf(os.Stderr, "  %s/intent.md\n", projectName)
	fmt.Fprintf(os.Stderr, "\nNext steps:\n")
	fmt.Fprintf(os.Stderr, "  cd %s\n", projectName)
	fmt.Fprintf(os.Stderr, "  Edit intent.md          — define your project's north star\n")
	fmt.Fprintf(os.Stderr, "  aglet new block <Name>   — add your first Block\n")
	fmt.Fprintf(os.Stderr, "  aglet new surface <Name> — add a frontend Surface\n")
	fmt.Fprintf(os.Stderr, "\n")
}

// rootDomainYAML produces the content of the root domain.yaml for a new project.
// Includes default runners for Go, TypeScript, and Python.
// Providers are shown as commented-out examples so the user can uncomment and fill in keys.
// The model line is included if provided via --model, otherwise shown as a commented example.
func rootDomainYAML(name, model string) string {
	// Build the defaults block — include model line if provided, else leave as comment
	var defaultsLines strings.Builder
	defaultsLines.WriteString("defaults:\n")
	defaultsLines.WriteString("  execution: sync\n")
	defaultsLines.WriteString("  error: propagate\n")
	if model != "" {
		defaultsLines.WriteString(fmt.Sprintf("  model: %s\n", model))
	} else {
		defaultsLines.WriteString("  # model: claude-sonnet-4-20250514  # Default LLM for reasoning Blocks\n")
	}

	uuid := generateTypedUUID("d")

	return fmt.Sprintf(`id: %s
name: %s

# runners tells the CLI how to execute each file type.
# Add a new language by adding one line here.
runners:
  .go: "go run"
  .ts: "npx tsx"
  .py: "python3"

# providers configures LLM access for reasoning Blocks.
# Uncomment and set the env variable for each provider you use.
# providers:
#   anthropic:
#     env: ANTHROPIC_API_KEY
#   openai:
#     env: OPENAI_API_KEY

%s`, uuid, name, defaultsLines.String())
}

// rootDomainIntent produces the content of the root intent.md for a new project.
func rootDomainIntent(name string) string {
	return fmt.Sprintf(`# %s

TODO: What is this application? Who is it for? What does it do?

## Sacred Constraints

TODO: Things that must never be compromised, no matter what.

## Who This Serves

TODO: The intended audience and their needs.
`, name)
}

// printInitUsage prints help for `aglet init`.
func printInitUsage() {
	fmt.Fprintf(os.Stderr, `Usage: aglet init <ProjectName> [flags]

Bootstraps a new Aglet project with a root domain.

Flags:
  --model <model>   Default LLM model for reasoning Blocks
                    (e.g. claude-sonnet-4-20250514, gpt-4o)

Examples:
  aglet init my-app
  aglet init my-app --model claude-sonnet-4-20250514

`)
}
