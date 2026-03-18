---
title: Blocks
---

# Blocks

A Block is a directory containing a `block.yaml` file. That file's presence is what makes a directory a Block. Each Block is a single-responsibility unit of logic: it receives typed JSON input, transforms it, and produces typed JSON output. Blocks are stateless — they never hold or mutate state between invocations.

Blocks are self-describing. Any infrastructure that can read `block.yaml`, send JSON matching the input schema, and receive JSON matching the output schema can host that Block. The `aglet` CLI is a development toolkit, not a runtime authority.

## When to Use Each Runtime

Every Block declares one of three runtimes. The runtime determines how the Block executes, and choosing the right one is the first design decision for each unit of logic.

### Process

`runtime: process` — the standard Block. A script that runs as its own process: JSON in via stdin, JSON out via stdout. Any language works — Go, Python, TypeScript — because the runner is configured in `domain.yaml`.

**Use when:** you can write a clear algorithm. Data parsing, API calls, calculations, file processing, anything with deterministic logic. Process Blocks are the workhorse.

A process Block directory contains:
- `block.yaml` — Identity, schemas, edges, behavioral policy.
- `intent.md` — Why this Block exists, design decisions, open questions.
- `main.*` — Implementation file. Language declared via `impl` in `block.yaml`.
- `logs.jsonl` — Runtime logs. Auto-generated, never hand-edited.

### Reasoning

`runtime: reasoning` — the implementation is a natural language prompt (`prompt.md`) rather than code. No `main.*` file. The prompt *is* the implementation.

**Use when:** you'd need a massive branching tree of conditionals to approximate what a human would intuitively understand. Classification, summarization, extraction, judgment calls.

The `aglet-reason` runner reads `prompt.md` as the system prompt, reads the output schema from `block.yaml` for structured output enforcement, receives JSON on stdin, makes the LLM API call, and writes structured JSON to stdout. If `tools` are declared, it resolves them from neighboring Blocks and handles tool-use loops automatically.

A reasoning Block directory contains:
- `block.yaml` — Identity, runtime, model, prompt pointer, tools, edges, schemas.
- `intent.md` — Why this is a reasoning task, what judgment it encodes.
- `prompt.md` — The implementation. The LLM's system prompt.

### Embedded

`runtime: embedded` — a pure importable function inside a Surface's runtime. Same design layer as a process Block (`block.yaml`, `intent.md`, typed schemas), but bundled by the Surface's build system rather than executed as a separate process.

**Use when:** a Surface component needs transformation logic that should be typed, tested, and tracked separately — but calling a subprocess or API would be overkill. Formatters, validators, state reducers.

Embedded Blocks must be stateless. They can receive current state as input and return new state as output, but they never reach into the Surface's state directly. If an embedded Block needs to grow into a full subprocess, promote it to process by wrapping it in a thin `main.*` with stdin/stdout handling.

## block.yaml Schema

### Identity Fields

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Typed UUID with `b-` prefix. Generated once, never changes. |
| `name` | Yes | PascalCase. Must match the folder name. |
| `description` | Yes | One-line summary for CLI output and quick scanning. |
| `domain` | Yes | Which domain this Block belongs to. |
| `role` | Yes | Semantic label: `transformer`, `classifier`, `verifier`, `gateway`, `emitter`, etc. Not a closed set — use whatever describes the shape of work. |

### Runtime Fields

| Field | Required | Description |
|-------|----------|-------------|
| `runtime` | No | `process` (default), `embedded`, or `reasoning`. |

### File Fields

| Field | Context | Description |
|-------|---------|-------------|
| `impl` | Process / Embedded | Path to implementation file, relative to Block directory. e.g., `./main.py`. |
| `prompt` | Reasoning | Path to prompt file. Defaults to `./prompt.md`. |

### Schema Fields

```yaml
schema:
  in:
    type: object
    properties:
      # JSON Schema draft-07 in YAML syntax
    required: [...]
  out:
    type: object
    properties:
      # JSON Schema draft-07 in YAML syntax
    required: [...]
```

`schema.in` defines what the Block accepts. `schema.out` defines what it emits. Tooling validates data flowing between Blocks by checking outputs against the downstream Block's `schema.in`.

### Edge Fields

| Field | Context | Description |
|-------|---------|-------------|
| `calls` | All runtimes | Forward data flow declarations. List of Block names this Block's output feeds into. |
| `tools` | Reasoning only | Blocks the LLM can invoke *during* reasoning. Only process and reasoning Blocks — never embedded. |

`calls` and `tools` serve different purposes. `tools` are invoked mid-reasoning (the LLM calls a tool Block, gets the result, continues thinking). `calls` are downstream edges — where the Block's final output goes *after* execution. The wrapper uses `calls` to propagate output through pipelines.

### Observe Fields

```yaml
observe:
  log: ./logs.jsonl
  events: [start, complete, error]
```

| Field | Required | Description |
|-------|----------|-------------|
| `observe.log` | No | Path to log file. Default: `./logs.jsonl`. |
| `observe.events` | No | Which events the wrapper logs. Default: all events. Reasoning blocks should include `tool.call`. |

The observe section is the Block's observability contract. The wrapper reads this and only logs events the Block opts into. Any execution environment that implements the wrapper protocol honors the same declaration. See [Runtime Architecture](/spec/runtime) for how wrappers use this.

### Behavior Fields

| Field | Context | Values | Description |
|-------|---------|--------|-------------|
| `execution` | Process | `sync`, `async` | Execution mode. Inheritable from domain defaults. |
| `error` | Process | `propagate`, `absorb`, `halt` | Error handling policy. Inheritable from domain defaults. |

### Reasoning-Specific Fields

| Field | Required | Description |
|-------|----------|-------------|
| `model` | No | LLM model identifier. Cascades: Block > domain defaults > root domain defaults. |
| `provider` | No | Explicit provider override. If omitted, inferred from model name (`claude-*` → `anthropic`, `gpt-*` → `openai`). |

## Implementation Contract

Every process Block follows the same interface regardless of language:

1. Read JSON from stdin
2. Transform the data
3. Write JSON to stdout
4. Exit 0 on success, non-zero on failure

**stdout is for data, stderr is for diagnostics.** Never mix error messages into the stdout JSON stream. The wrapper captures stderr and writes it to `logs.jsonl`.

The implementation has three labeled sections — **In**, **Transform**, **Out** — clearly marked with comments. The transformation logic lives in a separate function named after the Block.

**Go:**
```go
func main() {
    // In
    var input map[string]interface{}
    json.NewDecoder(os.Stdin).Decode(&input)

    // Transform
    result := PaymentAuth(input)

    // Out
    json.NewEncoder(os.Stdout).Encode(result)
}

func PaymentAuth(input map[string]interface{}) map[string]interface{} {
    authorized := verifyToken(input["token"].(string))
    return map[string]interface{}{
        "authorized": authorized,
        "transaction_id": generateId(),
    }
}
```

**Python:**
```python
import json
import sys

def greeter(data):
    name = data.get("name", "World")
    return {"greeting": f"Hello, {name}!", "length": len(name)}

# In
input_data = json.load(sys.stdin)

# Transform
result = greeter(input_data)

# Out
json.dump(result, sys.stdout)
```

**TypeScript:**
```typescript
import { readFileSync } from "fs";

// In
const input = JSON.parse(readFileSync("/dev/stdin", "utf-8"));

// Transform
const result = PaymentAuth(input);

// Out
console.log(JSON.stringify(result));

function PaymentAuth(input: any) {
    const authorized = verifyToken(input.token);
    return { authorized, transaction_id: generateId() };
}
```

## prompt.md

`prompt.md` is the implementation file for reasoning Blocks. It contains the system prompt — instructions, decision framework, and constraints that guide the LLM's reasoning.

Write it as clear instructions: what the input represents, what judgment to make, what the output should contain, what constraints to respect. The LLM receives the prompt (system message), the input (user message), and the output schema (structured output constraint).

```markdown
# Sentiment Analyzer

You are a sentiment analysis system. You receive a text input
and determine its emotional sentiment.

## Classification Rules

- **sentiment**: One of "positive", "negative", or "neutral"
- **confidence**: A number between 0 and 1
- **reasoning**: A brief explanation of your classification

## Constraints

- Be precise with confidence scores. Don't default to 0.5.
- Mixed sentiment: classify by dominant tone.
- Sarcasm: classify by intended meaning, not surface words.
```

## Naming Resolution

Block references in `calls` and `tools` use adaptive qualification — the shortest unambiguous name:

- One `Validator` in the project: write `Validator`.
- Two `Validator` Blocks in different domains: qualify as `auth/Validator` and `schema/Validator`.
- `aglet validate` checks names and reports which qualification is needed.

## Sizing a Block

A Block should do one thing. If you find yourself putting "and" in the description ("parses input *and* validates it *and* enriches it"), that's three Blocks. If the `intent.md` has multiple unrelated sections, the Block is too big.

The test: could someone understand what this Block does from its name, description, and schemas alone — without reading the implementation? If yes, it's the right size. If no, it's doing too much.

## Full Examples

### Process Block

```yaml
id: b-7c9e6679-7425-40de-944b-e07fc1f90ae7
name: PaymentAuth
description: "Verifies payment authorization tokens"
domain: auth
role: verifier
runtime: process
impl: ./main.go

schema:
  in:
    type: object
    properties:
      token: { type: string }
      session_id: { type: string }
      amount_cents: { type: integer }
    required: [token, session_id, amount_cents]
  out:
    type: object
    properties:
      authorized: { type: boolean }
      transaction_id: { type: string }
    required: [authorized, transaction_id]

calls:
  - SessionManager
  - payments/PaymentGateway

observe:
  log: ./logs.jsonl
  events: [start, complete, error]

execution: sync
error: propagate
```

### Reasoning Block

```yaml
id: b-9f8e7d6c-5b4a-3c2d-1e0f-a9b8c7d6e5f4
name: EmailClassifier
description: "Classifies emails by type"
domain: intelligence
role: classifier
runtime: reasoning
model: claude-sonnet-4-20250514
prompt: ./prompt.md

schema:
  in:
    type: object
    properties:
      email_body: { type: string }
      sender: { type: string }
      subject: { type: string }
    required: [email_body]
  out:
    type: object
    properties:
      category:
        type: string
        enum: [relational, action, attention, systemic]
      confidence: { type: number }
      reasoning: { type: string }
    required: [category, confidence]

tools:
  - ParseDate
  - ExtractEntities

calls:
  - TensionTracker

observe:
  log: ./logs.jsonl
  events: [start, complete, error, tool.call]
```

### Embedded Block

```yaml
id: b-a1b2c3d4-e5f6-7890-abcd-000000000001
name: StripSignature
description: "Removes email signatures from message bodies"
domain: messages
role: transformer
runtime: embedded
impl: ./main.ts

schema:
  in:
    type: object
    properties:
      body: { type: string }
    required: [body]
  out:
    type: object
    properties:
      cleanBody: { type: string }
      signatureFound: { type: boolean }
    required: [cleanBody, signatureFound]
```
