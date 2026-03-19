---
title: Getting Started
---

# Getting Started

This guide walks you through creating an Aglet project, building your first Blocks, connecting them in a pipeline, and seeing the observability layer in action. By the end, you'll have a working system that describes itself.

## Install

```bash
go install github.com/trevorschoeny/aglet@latest
```

Requires Go 1.22+. The binary installs to `~/go/bin/aglet`. Make sure `~/go/bin` is on your `PATH`.

## Create a Project

```bash
aglet init my-app
cd my-app
```

This creates a root domain with three files:
- `domain.yaml` — runners for Go, TypeScript, and Python; provider stubs; execution defaults
- `intent.md` — a template for your project's north star
- `CLAUDE.md` — agent context pointing to the full specification

Open `intent.md` and write a sentence about what your project does. This is the document that every unit in your project exists in service of.

## Create Your First Block

```bash
aglet new block Greeter --lang py
```

This creates `Greeter/` with a `block.yaml`, `intent.md`, and `main.py`.

### Define the Schema

Open `Greeter/block.yaml` and fill in the schemas:

```yaml
schema:
  in:
    type: object
    properties:
      name:
        type: string
    required: [name]
  out:
    type: object
    properties:
      greeting:
        type: string
      length:
        type: integer
    required: [greeting, length]
```

The schemas define the Block's contract with the outside world. Any infrastructure that can send JSON matching `schema.in` and receive JSON matching `schema.out` can use this Block.

### Implement the Block

Open `Greeter/main.py` and replace the TODO:

```python
import json
import sys


def greeter(input):
    name = input.get("name", "World")
    return {"greeting": f"Hello, {name}!", "length": len(name)}


# In
input_data = json.load(sys.stdin)

# Transform
result = greeter(input_data)

# Out
print(json.dumps(result))
```

The In/Transform/Out convention is universal: read JSON from stdin, transform it in a named function, write JSON to stdout. Any language, same pattern.

### Write the Intent

Open `Greeter/intent.md`:

```markdown
# Greeter

Produces a personalized greeting message. Exists as a simple demonstration
of the process Block pattern — stdin to stdout with typed schemas.

## Why This Exists

Entry point for learning Aglet. Shows that a Block is just a function
with metadata: JSON in, JSON out, identity in YAML.
```

### Run It

```bash
echo '{"name": "Aglet"}' | aglet run Greeter
```

Output:
```json
{"greeting": "Hello, Aglet!", "length": 5}
```

Behind the scenes, the wrapper observed the entire execution: logged `block.start` and `block.complete`, captured duration, checked for code changes, and updated the Block's vitals in `.aglet/`.

## Create a Second Block and Connect Them

```bash
aglet new block Shouter --lang py
```

Open `Shouter/block.yaml` and set the schema:

```yaml
schema:
  in:
    type: object
    properties:
      greeting:
        type: string
    required: [greeting]
  out:
    type: object
    properties:
      shout:
        type: string
    required: [shout]
```

Implement `Shouter/main.py`:

```python
import json
import sys


def shouter(input):
    return {"shout": input["greeting"].upper() + "!!!"}


# In
input_data = json.load(sys.stdin)

# Transform
result = shouter(input_data)

# Out
print(json.dumps(result))
```

Now connect them. Open `Greeter/block.yaml` and add a `calls` edge:

```yaml
calls:
  - Shouter
```

Run the pipeline:

```bash
echo '{"name": "Aglet"}' | aglet pipe Greeter
```

Output:
```json
{"shout": "HELLO, AGLET!!!!"}
```

The pipeline followed the `calls` edge from Greeter to Shouter automatically. Both Blocks were wrapped with full observability — each got their own log entries, vitals updates, and version tracking.

## See the Behavioral Memory

After a few runs, check the stats:

```bash
aglet stats Greeter
```

```
Block: Greeter
  Warmth:      hot (0.85)
  Total calls: 5
  Avg runtime:  12.4ms
  Error rate:   0.0%
  Last called:  2 minutes ago
```

The AML has been silently observing. Every run accumulated knowledge — call counts, timing, error rates, warmth. This data lives in `.aglet/Greeter/vitals.json`, readable by any agent or tool.

## Validate the Project

```bash
aglet validate
```

```
[aglet validate] Scanning project...
  Found 2 blocks, 0 surfaces, 0 components, 1 domain

  Checks: 12 passed, 0 failed
  Auto-fixed: 0

  ✓ Project is valid.
```

Validate checks structural integrity: UUIDs, name-folder agreement, intent files, domain references, schema presence, calls edges, and more. It auto-fixes what it can and reports what needs attention.

## Next Steps

You've created a project, built two Blocks, connected them in a pipeline, observed the behavioral layer, and validated the structure. Here's where to go next:

- **[How It Works](/guide/how-it-works)** — The full picture: wrappers, listeners, the AML, surface observability, and how everything connects.
- **[Blocks Specification](/spec/blocks)** — Deep reference for `block.yaml`, runtime types, and implementation patterns.
- **[Surfaces Specification](/spec/surfaces)** — Build frontends with typed contracts to your Block backend.
- **[Agent Setup](/guide/agent-setup)** — Set up your AI agent to leverage Aglet's metadata.
