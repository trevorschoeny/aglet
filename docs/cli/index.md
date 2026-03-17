---
title: CLI Reference
---

# CLI Reference

## Install

```bash
go install github.com/trevorschoeny/aglet@latest
```

Requires Go 1.22+. The binary installs to `~/go/bin/aglet`. Make sure `~/go/bin` is on your `PATH`.

## Project Discovery

The CLI finds the project root by walking up from your current working directory, looking for a root `domain.yaml` -- one without a `parent` field. Every command that operates on a project uses this mechanism.

---

## aglet run

Find and execute a single Block by name. The CLI scans the project tree for a `block.yaml` whose `name` field matches the argument.

```
aglet run <BlockName> [input.json]
```

**Process blocks** resolve their runner from the `runners` map in `domain.yaml` (keyed by file extension). **Reasoning blocks** use the built-in `aglet-reason` runner, which calls the LLM API directly. **Embedded blocks** are rejected with an error -- they are internal to Surfaces and cannot be executed externally.

### Input

Input is resolved in this order:

1. File argument (`input.json`) -- read from disk
2. Stdin pipe -- read if stdin is not a TTY
3. Default -- empty JSON `{}`

### Examples

```bash
# Pipe JSON from another command
echo '{"name": "world"}' | aglet run Greeter

# Read input from a file
aglet run SentimentAnalyzer input.json

# Run with empty input (defaults to {})
aglet run WordCount
```

---

## aglet reason

Execute a reasoning Block directly from its directory path. This skips the discovery scan -- you point it at the Block's folder instead of searching by name. Useful when iterating on prompts, since you don't need a fully wired project.

```
aglet reason <BlockDir> [input.json]
```

Only works on Blocks with `runtime: reasoning`. Returns an error for process or embedded Blocks.

### Examples

```bash
# Run from the Block's directory
aglet reason ./SentimentAnalyzer input.json

# Pipe input
echo '{"text": "This is great"}' | aglet reason ./SentimentAnalyzer
```

---

## aglet pipe

Execute a pipeline by following `calls` edges in `block.yaml`. Each Block's stdout feeds the next Block's stdin.

```
aglet pipe <StartBlock> [EndBlock]
```

**One argument:** follows `calls` edges linearly from the start Block to the terminal Block (one with no `calls`). Fails if the graph branches -- pipelines must be linear.

**Two arguments:** finds the shortest path (BFS) between the start and end Blocks in the calls graph.

Input is read the same way as `aglet run` (file, stdin, or empty JSON). If the last argument ends in `.json`, it is treated as an input file.

### Examples

```bash
# Follow calls edges to the terminal Block
aglet pipe FetchPage

# Shortest path between two Blocks
aglet pipe FetchPage Summarize

# With input file
aglet pipe FetchPage Summarize input.json

# With stdin
echo '{"url": "https://example.com"}' | aglet pipe FetchPage
```

---

## aglet serve

Start an HTTP dev server from a Surface's contract. Each contract dependency becomes a `POST /contract/<DependencyName>` endpoint. Blocks are also accessible directly at `POST /block/<BlockName>`.

```
aglet serve [--port PORT]
```

Default port is `3001`. CORS headers are included for local development (`Access-Control-Allow-Origin: *`).

If no `surface.yaml` is found, the server runs in direct mode -- all Blocks are exposed at `/block/{name}` without contract routing.

Contract dependencies can map to a single Block (`block` field) or a pipeline (`pipeline` field that follows `calls` edges from the named Block).

### Examples

```bash
# Start on default port 3001
aglet serve

# Start on a custom port
aglet serve --port 8080

# Then call an endpoint
curl -X POST http://localhost:3001/contract/Analyze \
  -H "Content-Type: application/json" \
  -d '{"text": "Hello world"}'
```

---

## aglet init

Bootstrap a new Aglet project. Creates a root domain directory with a `domain.yaml` and `intent.md`, ready to scaffold Blocks and Surfaces into.

```
aglet init <ProjectName> [--model <model>]
```

The generated `domain.yaml` includes default runners for Go, TypeScript, and Python, and a commented-out providers stub for easy LLM configuration.

### Flags

| Flag | Description |
|---|---|
| `--model <model>` | Set the default LLM model for reasoning Blocks (e.g. `claude-sonnet-4-20250514`) |

### Examples

```bash
aglet init my-app
aglet init my-app --model claude-sonnet-4-20250514
```

After running, edit `intent.md` to define your project's north star, then start adding units with `aglet new`.

---

## aglet new

Scaffold a new unit — Block, Domain, Surface, or Component. Creates the directory and all required files in one pass so every unit is born complete.

```
aglet new <type> <name> [flags]
```

The `domain` field is inferred automatically from the nearest ancestor `domain.yaml`. Run this command from inside the domain directory where the unit should live.

### Types

| Type | Creates |
|---|---|
| `block` | `block.yaml`, `intent.md`, `main.*` (or `prompt.md` for reasoning) |
| `domain` | `domain.yaml`, `intent.md` |
| `surface` | `surface.yaml`, `intent.md`, `main.tsx` |
| `component` | `component.yaml`, `intent.md`, `ComponentName.tsx` |

### Block flags

| Flag | Values | Default |
|---|---|---|
| `--runtime` | `process`, `embedded`, `reasoning` | `process` |
| `--lang` | `go`, `ts`, `py` | `go` (process), `ts` (embedded) |
| `--domain` | domain name | inferred from nearest `domain.yaml` |

### Domain flags

| Flag | Description |
|---|---|
| `--parent` | Parent domain name (default: inferred from nearest `domain.yaml`) |

### Surface / Component flags

| Flag | Description |
|---|---|
| `--domain` | Domain name (default: inferred from nearest `domain.yaml`) |

### Examples

```bash
# Process block (Go, default)
aglet new block FetchPage

# Reasoning block
aglet new block EmailClassifier --runtime reasoning

# Embedded block (TypeScript)
aglet new block StripSignature --runtime embedded

# Python process block
aglet new block ParseDate --lang py

# Domain, surface, component
aglet new domain intelligence
aglet new surface TrevMailClient
aglet new component ConversationList
```

---

## aglet validate

Check project integrity and auto-fix what it can. Scans all `block.yaml`, `surface.yaml`, `component.yaml`, and `domain.yaml` files in the project tree.

```
aglet validate
```

### What it checks

| Category | Checks |
|---|---|
| **UUIDs** | Present, correct format (`prefix-uuid`), correct prefix per unit type (`b-`, `s-`, `c-`, `d-`), unique across project |
| **Name/folder match** | Unit name in YAML matches its directory name |
| **Intent files** | Every unit directory has an `intent.md` |
| **Domain references** | Every unit's `domain` field references an existing `domain.yaml` |
| **Block files** | Valid `runtime` value, `impl` file exists (process/embedded), `schema.in` and `schema.out` present |
| **Reasoning blocks** | Model resolvable (block or domain default), `prompt.md` exists, tools reference valid blocks, no `main.*` file |
| **Calls edges** | Every `calls` entry references an existing Block |
| **Schema compatibility** | For each `calls` edge, every field required by the downstream Block's `schema.in` is present in the upstream Block's `schema.out`, with compatible types |
| **Circular deps** | No cycles in the calls graph (DFS) |
| **Surfaces** | Entry file exists, no nested surfaces, contract dependencies reference existing blocks/pipelines |
| **Components** | `consumes` entries exist in parent surface contract, bidirectional caller/consumes consistency |
| **Domains** | `parent` references an existing domain |

### Auto-fix behavior

These issues are fixed automatically:

| Issue | Fix |
|---|---|
| Name/folder mismatch | Updates the YAML `name` field to match the folder |
| Missing `intent.md` | Creates a stub `intent.md` with a TODO placeholder |
| Missing `prompt.md` (reasoning) | Creates a stub `prompt.md` with a TODO placeholder |
| Bidirectional caller mismatch | Adds missing `consumes` entry or `callers` entry |
| Invalid domain `parent` | Infers parent from filesystem nesting if an ancestor domain exists |

### Not auto-fixable

These require manual intervention:

- Missing or malformed UUIDs
- Missing `impl` file or `main.*` file
- Missing `schema.in` or `schema.out`
- Invalid `runtime` value
- Broken `calls` references
- Schema compatibility mismatches (missing fields or type conflicts between connected Blocks)
- Circular dependencies
- Nested surfaces
- Missing model with no domain default

### Example output

```
[aglet validate] Scanning project...
[aglet validate] Found 3 blocks, 1 surfaces, 2 components, 2 domains

  ✔ Fixed: Greeter → name updated to 'Greeter'
  ✔ Fixed: Greeter → created stub intent.md

  SentimentAnalyzer
    ✗ missing schema.out in block.yaml
  ClassifyEmail
    ✗ schema mismatch with 'ScoreEmail': output is missing field 'score' required by ScoreEmail.schema.in

[aglet validate] 1 issue(s) fixed, 2 error(s) remaining
```

---

## Architecture notes

The `aglet` CLI is a single Go binary with zero dependencies beyond `gopkg.in/yaml.v3`. It contains no LLM SDKs -- it speaks HTTP directly to provider APIs.

Supported provider formats:
- **Anthropic** -- Messages API (`/v1/messages`), uses tool-use pattern for structured output
- **OpenAI** -- Chat Completions API (`/v1/chat/completions`), uses `json_schema` response format for structured output

Providers and their API keys are configured in the root `domain.yaml`. The CLI resolves which provider to use based on the model name and optional `provider` field in `block.yaml`.
