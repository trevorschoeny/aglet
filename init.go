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

	// Write CLAUDE.md — the agent context file that tells AI agents about this project
	claudePath := filepath.Join(projectDir, "CLAUDE.md")
	if err := os.WriteFile(claudePath, []byte(rootCLAUDEMd(projectName)), 0644); err != nil {
		os.RemoveAll(projectDir)
		fmt.Fprintf(os.Stderr, "Error: could not write CLAUDE.md: %s\n", err)
		os.Exit(1)
	}

	// Write .gitignore — exclude .aglet/ runtime data from main repo
	gitignorePath := filepath.Join(projectDir, ".gitignore")
	gitignoreContent := "# Aglet runtime data (logs + behavioral memory) — has its own git repo\n.aglet/\n\n# Common excludes\nnode_modules/\ndist/\n"
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write .gitignore: %s\n", err)
	}

	// Initialize .aglet/ runtime data directory with its own git repo
	if err := InitAgletRepo(projectDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not initialize .aglet/: %s\n", err)
	}

	// Report what was created
	fmt.Fprintf(os.Stderr, "[aglet init] Created project '%s'\n", projectName)
	fmt.Fprintf(os.Stderr, "  %s/domain.yaml\n", projectName)
	fmt.Fprintf(os.Stderr, "  %s/intent.md\n", projectName)
	fmt.Fprintf(os.Stderr, "  %s/CLAUDE.md\n", projectName)
	fmt.Fprintf(os.Stderr, "  %s/.gitignore\n", projectName)
	fmt.Fprintf(os.Stderr, "  %s/.aglet/              (runtime data — own git repo)\n", projectName)
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

# stores declares database connections available to Blocks in this domain.
# The wrapper injects AGLET_STORE_{NAME} env vars into process Blocks at runtime.
# Use ${ENV_VAR} references so secrets stay out of YAML.
# stores:
#   main:
#     driver: postgres
#     dsn: ${DATABASE_URL}

%s
aglet:
  sink: local
`, uuid, name, defaultsLines.String())
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

// rootCLAUDEMd produces the CLAUDE.md for a new project.
// This file is the agent context file — it tells AI agents (Claude Code, etc.) that
// this is an Aglet project and where to find the full specification.
// It's intentionally lean: the full spec lives at the docs URL, not embedded here.
func rootCLAUDEMd(name string) string {
	bt := "`"
	fence := "```"
	return fmt.Sprintf("# %s\n"+
		"\n"+
		"This is an [Aglet](https://github.com/trevorschoeny/aglet) project.\n"+
		"\n"+
		"Aglet is a protocol for self-describing, agent-native computation. Applications are composed of **Blocks** (stateless logic units), **Surfaces** (stateful frontends), **Components** (UI building blocks), and **Domains** (organizational groupings). Every unit carries an identity file (block.yaml, surface.yaml, etc.) and an intent.md explaining why it exists.\n"+
		"\n"+
		"## Full Specification\n"+
		"\n"+
		"https://trevorschoeny.github.io/aglet/\n"+
		"\n"+
		"Read the spec before creating, modifying, or scaffolding any Blocks, Surfaces, Components, or Domains. The spec covers unit types, file schemas, runtime behaviors, the contract system, and the Adaptive Memory Layer (AML).\n"+
		"\n"+
		"## Quick Reference\n"+
		"\n"+
		"- **Block** — a directory with %sblock.yaml%s. JSON in, JSON out. Three runtimes: process, embedded, reasoning.\n"+
		"- **Surface** — a directory with %ssurface.yaml%s. A deployable frontend with a typed contract to its Block dependencies.\n"+
		"- **Component** — a directory with %scomponent.yaml%s. A stateful unit inside a Surface.\n"+
		"- **Domain** — a directory with %sdomain.yaml%s. Organizational grouping; carries config that children inherit.\n"+
		"- **intent.md** — every unit has one. The *why* document. Read it before touching anything.\n"+
		"\n"+
		"## CLI\n"+
		"\n"+
		"Install: %sgo install github.com/trevorschoeny/aglet@latest%s\n"+
		"\n"+
		"%sbash\n"+
		"aglet run <BlockName>          # Execute a Block by name\n"+
		"aglet pipe <StartBlock>        # Execute a pipeline (follows calls edges)\n"+
		"aglet serve                    # HTTP dev server for Surface development\n"+
		"aglet validate                 # Check project structure, auto-fix what it can\n"+
		"aglet validate --deep          # Generate judgment-based checklist for agent review\n"+
		"aglet stats <BlockName>        # Behavioral memory from runtime logs\n"+
		"aglet new block <Name>         # Scaffold a new Block\n"+
		"aglet new surface <Name>       # Scaffold a new Surface\n"+
		"%s\n",
		name,
		bt, bt, bt, bt, bt, bt, bt, bt, // 8 backtick pairs for inline code
		bt, bt,    // install line
		fence, fence, // code block
	)
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
