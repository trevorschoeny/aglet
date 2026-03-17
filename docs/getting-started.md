# Getting Started

## Install the CLI

```bash
go install github.com/trevorschoeny/aglet@latest
```

Make sure `~/go/bin` is in your `PATH`.

## Create a project

An Aglet project starts with a root domain:

```bash
mkdir my-project && cd my-project
```

Create `domain.yaml`:

```yaml
id: d-YOUR-UUID-HERE
name: my-project
providers:
  anthropic:
    env: ANTHROPIC_API_KEY
defaults:
  model: claude-sonnet-4-20250514
```

Create `intent.md`:

```markdown
# my-project

This project does X for Y because Z.
```

## Add a Block

Create a directory with a `block.yaml` and an implementation:

```bash
mkdir Greeter
```

`Greeter/block.yaml`:
```yaml
id: b-GENERATE-A-UUID
name: Greeter
description: "Greets a user by name"
domain: my-project
runtime: process
impl: ./main.py
schema:
  in:
    type: object
    properties:
      name: { type: string }
    required: [name]
  out:
    type: object
    properties:
      greeting: { type: string }
    required: [greeting]
```

`Greeter/intent.md`:
```markdown
# Greeter

A simple Block that produces a greeting. Exists as a starting point for learning the Aglet protocol.
```

`Greeter/main.py`:
```python
import json, sys

def Greeter(data):
    name = data.get("name", "world")
    return {"greeting": f"Hello, {name}!"}

if __name__ == "__main__":
    data = json.load(sys.stdin)
    result = Greeter(data)
    json.dump(result, sys.stdout)
```

## Run it

```bash
echo '{"name": "Trevor"}' | aglet run Greeter
# → {"greeting": "Hello, Trevor!"}
```

## Validate

```bash
aglet validate
# [aglet validate] All checks passed
```

## Set up your agent

Add a `CLAUDE.md` (or equivalent for your agent) to the project root:

```markdown
This is an Aglet project. See https://github.com/trevorschoeny/aglet for the full specification.
```

Your agent will read the project's YAML files and intent documents to understand the system, and use `aglet validate` to catch structural drift.
