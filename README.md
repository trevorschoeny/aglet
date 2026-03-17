# Aglet

A protocol for observable, agent-native software — semantic identity, traceability, and analytics built into every unit.

## What is Aglet?

Aglet is a protocol for self-describing computation. Applications are composed of **units** that carry everything needed to execute them — identity, intent, typed schemas, and implementation — organized within domains and governed by founding intent documents.

Every unit has a YAML identity file, a natural-language intent document, and typed input/output schemas. This means every piece of your system is **discoverable** (agents can find and understand it), **traceable** (you know what calls what), and **observable** (analytics are a natural consequence of the structure, not an afterthought).

The core protocol is simple: any infrastructure that can read a Block's `block.yaml`, send JSON matching the input schema, and receive JSON matching the output schema can host that Block. Blocks are self-describing and self-sufficient — they don't depend on a central orchestrator.

## The Taxonomy

Aglet has five core concepts:

| Concept | Identity File | What it is |
|---------|--------------|------------|
| **Block** | `block.yaml` | A stateless, single-responsibility unit of logic. JSON in, JSON out. |
| **Surface** | `surface.yaml` | A stateful application — the boundary where the system meets the outside world. |
| **Component** | `component.yaml` | A stateful unit within a Surface. Handles orchestration, state, and user events. |
| **Domain** | `domain.yaml` | An organizational grouping. Carries configuration defaults and defines scope. |
| **Intent** | `intent.md` | Every unit has one. The document that explains *why* it exists. |

### Blocks

Blocks come in three runtimes:

- **Process** (`runtime: process`) — A standalone process. Reads JSON from stdin, writes JSON to stdout. Any language works.
- **Reasoning** (`runtime: reasoning`) — The implementation is a natural language prompt (`prompt.md`). Executed by the built-in `aglet-reason` runner, which makes LLM API calls with structured input/output schemas and handles tool-use loops.
- **Embedded** (`runtime: embedded`) — Imported within a Surface as pure functions. Same design layer, but bundled by the Surface's build system.

### Example Block

```
FetchPage/
├── block.yaml     # Identity, schemas, edges
├── intent.md      # Why this Block exists
└── main.py        # Implementation
```

```yaml
# block.yaml
id: b-b1a2c3d4-e5f6-7890-abcd-200000000001
name: FetchPage
description: "Fetches a URL and extracts the page title and text content"
domain: pipeline
role: gateway
runtime: process
impl: ./main.py
calls:
  - TagBookmark
schema:
  in:
    type: object
    properties:
      url:
        type: string
    required: [url]
  out:
    type: object
    properties:
      url: { type: string }
      title: { type: string }
      content: { type: string }
    required: [url, title, content]
```

## The CLI

The `aglet` CLI is a development toolkit — not a runtime authority.

### Install

```bash
go install github.com/trevorschoeny/aglet@latest
```

### Commands

```bash
aglet init <ProjectName>                # Bootstrap a new Aglet project
aglet new <type> <name> [flags]         # Scaffold a Block, Domain, Surface, or Component
aglet run <BlockName> [input.json]      # Execute a Block by name
aglet reason <BlockDir> [input.json]    # Execute a reasoning Block directly
aglet pipe <StartBlock> [EndBlock]      # Execute a pipeline following calls edges
aglet serve [--port PORT]               # Start HTTP dev server from a Surface's contract
aglet stats [BlockName] [flags]         # Behavioral memory from logs (the AML layer)
aglet validate                          # Check project integrity and auto-fix issues
aglet version                           # Print the installed version
```

### `aglet init` and `aglet new`

```bash
# Bootstrap a new project
aglet init my-app

# Scaffold units — domain is inferred from your current directory
cd my-app
aglet new domain intelligence
cd intelligence
aglet new block EmailClassifier --runtime reasoning
aglet new block ScoreEmail
```

### `aglet validate`

Scans the entire project, checks structural integrity, and auto-fixes what it can — no flags needed.

```
$ aglet validate
[aglet validate] Scanning project...
[aglet validate] Found 5 blocks, 1 surface, 4 components, 3 domains

  ✔ Fixed: BookmarkClient (surface) → name updated to 'client'
  ✔ Fixed: BookmarkCard (component) → added 'DeleteBookmark' to consumes

[aglet validate] 2 issue(s) found and fixed
```

Auto-fixes include: name/folder mismatches, missing `intent.md` stubs, bidirectional contract drift, and domain parent inference from filesystem nesting. Schema compatibility between connected Blocks is also checked — field presence and type mismatches across `calls` edges are flagged.

## Project Structure

An Aglet project is a directory tree where YAML identity files define what each directory is:

```
my-project/
├── domain.yaml              # Root domain (project-level config)
├── intent.md                # Founding intent for the whole system
├── pipeline/
│   ├── domain.yaml          # Sub-domain
│   ├── intent.md
│   ├── FetchPage/
│   │   ├── block.yaml
│   │   ├── intent.md
│   │   └── main.py
│   └── TagBookmark/
│       ├── block.yaml
│       ├── intent.md
│       └── main.py
└── client/
    ├── surface.yaml         # Surface with inline contract
    ├── intent.md
    └── bookmarks/
        ├── domain.yaml
        ├── BookmarkList/
        │   ├── component.yaml
        │   └── BookmarkList.tsx
        └── BookmarkCard/
            ├── component.yaml
            └── BookmarkCard.tsx
```

## Agent-Native Development

Aglet is designed to be used with whatever AI coding agent you already have — Claude Code, Cursor, Copilot, or anything else. The protocol gives your agent everything it needs to understand, navigate, and modify your system:

- **`block.yaml`** tells the agent what a unit does, what it expects, and what it produces
- **`intent.md`** tells the agent *why* a unit exists and what design decisions matter
- **`calls`** edges tell the agent how units connect
- **`surface.yaml` contracts** tell the agent what data the frontend needs and which components consume it
- **`aglet validate`** catches structural drift so the agent (or you) can fix it immediately

To start an Aglet project with Claude Code, add a `CLAUDE.md` to your project root:

```markdown
This is an Aglet project. See https://github.com/trevorschoeny/aglet for the full specification.
```

## Full Specification

The complete Aglet specification — including file schemas, structural guardrails, development workflows, and the sync check system — is available in the [docs](docs/).

## License

MIT
