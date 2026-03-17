---
title: Getting Started
---

# Getting Started

This guide walks you through installing Aglet, creating a project, building your first Block, and running it.

## Install the CLI

Aglet is a single Go binary with no external dependencies.

```bash
go install github.com/trevorschoeny/aglet@latest
```

Make sure `~/go/bin` is in your `PATH`. Verify the install:

```bash
aglet version
```

## Create a Project

Bootstrap a new Aglet project with `aglet init`:

```bash
aglet init my-project
cd my-project
```

This creates two files:

```
my-project/
├── domain.yaml   # Root domain: runners, defaults, providers stub
└── intent.md     # Founding document placeholder
```

Open `domain.yaml` and uncomment the providers section for your LLM:

```yaml
id: d-...
name: my-project

runners:
  .go: "go run"
  .ts: "npx tsx"
  .py: "python3"

providers:
  anthropic:
    env: ANTHROPIC_API_KEY   # uncomment and set your key

defaults:
  execution: sync
  error: propagate
  # model: claude-sonnet-4-20250514
```

Then open `intent.md` and replace the placeholder with your project's north star — one paragraph explaining what this system does and who it serves. Every decision you make should trace back to it.

## Create Your First Block

Scaffold a Block with `aglet new`:

```bash
aglet new block Greeter --lang py
```

This creates:

```
Greeter/
├── block.yaml   # Identity, schema, runtime config
├── intent.md    # Why this Block exists
└── main.py      # Implementation stub
```

Open `Greeter/block.yaml` and fill in the schema:

```yaml
id: b-...
name: Greeter
description: "Greets a person by name"
domain: my-project
role: transformer
runtime: process
impl: ./main.py

schema:
  in:
    type: object
    properties:
      name:
        type: string
    required:
      - name
  out:
    type: object
    properties:
      greeting:
        type: string
      length:
        type: integer
    required:
      - greeting
      - length
```

The `schema` section declares the Block's typed input/output contract using JSON Schema, inline in the YAML. The `runtime: process` means this Block reads JSON from stdin and writes JSON to stdout.

Open `Greeter/intent.md` and write why this Block exists:

```markdown
# Greeter

Takes a person's name and produces a personalized greeting.
```

Then replace the stub in `Greeter/main.py` with the real implementation:

```python
import json
import sys

# In
input_data = json.load(sys.stdin)

# Transform
result = Greeter(input_data)

# Out
json.dump(result, sys.stdout)


def Greeter(data):
    name = data.get("name", "World")
    return {"greeting": f"Hello, {name}!", "length": len(name)}
```

The implementation follows a simple pattern: read JSON from stdin, transform it, write JSON to stdout. Any language works — Python, Go, TypeScript, Rust, shell scripts. As long as it reads stdin and writes stdout, Aglet can run it.

## Run It

```bash
echo '{"name": "Trevor"}' | aglet run Greeter
```

```json
{"greeting": "Hello, Trevor!", "length": 6}
```

`aglet run` scans the project for a Block named `Greeter`, finds its `block.yaml`, resolves the runner for `.py` files from `domain.yaml`, and executes `python3 main.py` with your JSON piped to stdin. The output is the Block's stdout.

You can also pass input as a file:

```bash
aglet run Greeter input.json
```

## Validate Your Project

```bash
aglet validate
```

This scans every unit in the project and checks for structural issues: missing `intent.md` files, name/folder mismatches, broken `calls` references, invalid UUIDs, circular dependencies, schema compatibility between connected Blocks, and more. Some issues are auto-fixed (like creating a missing `intent.md` stub). Others require manual attention.

Run validate early and often. It catches design-layer drift before it becomes a problem.

## Set Up Your Agent

If you're working with an AI coding agent, add a `CLAUDE.md` (or equivalent config file) to your project root:

```markdown
This is an Aglet project -- load the Aglet specification from
https://github.com/trevorschoeny/aglet
```

This gives your agent the context it needs to understand the project structure, read `block.yaml` schemas, follow `calls` edges, and generate code that conforms to the protocol. See [Agent Setup](./agent-setup.md) for the full guide.

## What's Next

- [Core Concepts](./core-concepts.md) -- deep dive on Blocks, Surfaces, Components, and Domains
- [Agent Setup](./agent-setup.md) -- configuring your AI agent for Aglet projects
