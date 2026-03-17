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
aglet
```

You should see the command list: `run`, `reason`, `pipe`, `serve`, `validate`.

## Create a Project

An Aglet project starts with a root domain. Create a directory and add two files:

```bash
mkdir my-project && cd my-project
```

**`domain.yaml`** -- the root domain config:

```yaml
id: d-a1b2c3d4-e5f6-7890-abcd-ef1234567890
name: my-project

runners:
  .py: "python3"
  .ts: "npx tsx"

providers:
  anthropic:
    env: ANTHROPIC_API_KEY

defaults:
  execution: sync
  error: propagate
  model: claude-sonnet-4-20250514
```

The `runners` map file extensions to the commands that execute them. The `providers` section configures LLM API access for reasoning Blocks. The `defaults` section sets inheritable values for all units in this domain.

**`intent.md`** -- why this project exists:

```markdown
# My Project

A short description of what this project does and why it exists.
```

Every unit in Aglet has an `intent.md`. For domains, it's the vision document. For Blocks, it's more focused.

## Create Your First Block

Blocks live in directories named after them. Create a Greeter Block:

```bash
mkdir Greeter
```

**`Greeter/block.yaml`**:

```yaml
id: b-11111111-2222-3333-4444-555555555555
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

The `schema` section declares the Block's input/output contract using JSON Schema, inline in the YAML. The `runtime: process` means this Block is a script that reads JSON from stdin and writes JSON to stdout.

**`Greeter/intent.md`**:

```markdown
# Greeter

Takes a person's name and produces a personalized greeting.
```

**`Greeter/main.py`**:

```python
import json
import sys

def Greeter(data):
    name = data.get("name", "World")
    return {"greeting": f"Hello, {name}!", "length": len(name)}

# In
input_data = json.load(sys.stdin)

# Transform
result = Greeter(input_data)

# Out
json.dump(result, sys.stdout)
```

The implementation follows a simple pattern: read JSON from stdin, transform it, write JSON to stdout. Any language works -- Python, Go, Node, Rust, shell scripts. As long as it reads stdin and writes stdout, Aglet can run it.

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

This scans every unit in the project and checks for structural issues: missing `intent.md` files, name/folder mismatches, broken `calls` references, invalid UUIDs, circular dependencies, and more. Some issues are auto-fixed (like creating a missing `intent.md` stub). Others require manual attention.

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
