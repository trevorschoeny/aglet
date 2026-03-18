---
title: Blocks
---

# Blocks

A Block is a directory containing a `block.yaml` file. That file's presence is what makes a directory a Block. Each Block is a single-responsibility unit of logic: it receives typed JSON input, transforms it, and produces typed JSON output. Blocks are stateless -- they never hold or mutate state between invocations.

Blocks are self-describing. Any infrastructure that can read a Block's `block.yaml`, send JSON matching the input schema, and receive JSON matching the output schema can host that Block. There is no central orchestrator. The `aglet` CLI is a development toolkit, not a runtime authority.

## Runtimes

Every Block declares one of three runtimes. The runtime determines how the Block is executed.

### Process

`runtime: process` -- the standard Block. Runs as its own process, reading JSON from stdin and writing JSON to stdout. Any language with a runner defined in `domain.yaml` works.

A process Block directory contains:

- `block.yaml` -- Identity, schemas, edges, behavioral policy.
- `intent.md` -- Why this Block exists, design decisions, open questions.
- `main.*` -- Implementation file. Language declared via `impl` in `block.yaml`.
- `logs.jsonl` -- Runtime logs. Auto-generated, never hand-edited.

### Embedded

`runtime: embedded` -- a pure importable function inside a Surface's runtime. Same design layer as a process Block (`block.yaml`, `intent.md`, typed schemas), but bundled by the Surface's build system rather than executed as a separate process. Invisible to external tooling -- the `aglet` CLI cannot execute embedded Blocks.

Embedded Blocks must be stateless. They can receive current state as input and return new state as output (the reducer pattern: `(currentState, event) => newState`), but they never reach into the Surface's state directly. No `setState`, no context reads, no side effects. If you need an embedded Block to mutate state, it should be a Component instead.

An embedded Block can be promoted to a process Block by wrapping it in a thin `main.*` with stdin/stdout handling. The intent, schemas, and transformation logic transfer directly.

### Reasoning

`runtime: reasoning` -- the implementation is a natural language prompt (`prompt.md`) rather than code. No `main.*` file. The prompt *is* the implementation.

Executed by the `aglet-reason` runner -- a language runtime for natural language, analogous to `python3` for `.py` files. It reads `prompt.md` as the system prompt, reads the output schema from `block.yaml` for structured output enforcement, receives JSON on stdin, makes the LLM API call, and writes structured JSON to stdout. If `tools` are declared, it resolves them from neighboring Blocks and handles tool-use loops.

A reasoning Block directory contains:

- `block.yaml` -- Identity, runtime, model, prompt pointer, tools, edges, schemas.
- `intent.md` -- Why this is a reasoning task, what judgment it encodes.
- `prompt.md` -- The implementation. The LLM's system prompt.

No `main.*` file. No `logs.jsonl` (the runner handles logging).

**When to use reasoning vs. process:** If you can write a clear algorithm, it's a process Block. If you'd need a massive branching tree of conditionals to approximate what a human would intuitively understand, it's a reasoning Block.

## block.yaml Schema

`block.yaml` carries a Block's identity, runtime configuration, edges, behavioral policy, and input/output schemas. All schemas use JSON Schema draft-07, expressed in YAML syntax.

### Identity Fields

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Typed UUID with `b-` prefix. Generated once, never changes. Format: `b-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`. |
| `name` | Yes | PascalCase. Must match the folder name. How other Blocks reference this one. |
| `description` | Yes | One-line summary. Used in CLI output, graph labels, quick scanning. |
| `domain` | Yes | Which domain this Block belongs to. Must match a `domain.yaml` name in the project. |
| `role` | Yes | What shape of work this Block does: `transformer`, `classifier`, `verifier`, `gateway`, `emitter`, etc. Not a closed set. |

### Runtime Fields

| Field | Required | Description |
|-------|----------|-------------|
| `runtime` | No | `process` (default), `embedded`, or `reasoning`. |

### File Fields

| Field | Context | Description |
|-------|---------|-------------|
| `impl` | Process / Embedded | Path to the implementation file, relative to the Block directory. e.g., `./main.py`. |
| `prompt` | Reasoning | Path to the prompt file. Defaults to `./prompt.md`. |

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
| `tools` | Reasoning only | Blocks the LLM can invoke *during* reasoning. Only process and reasoning Blocks -- never embedded. |

`calls` and `tools` serve different purposes. `tools` are invoked mid-reasoning (the LLM decides it needs data, calls a tool Block, gets the result, continues thinking). `calls` are downstream edges -- where the Block's final output goes *after* execution.

### Behavior Fields

| Field | Context | Values | Description |
|-------|---------|--------|-------------|
| `execution` | Process | `sync`, `async` | Execution mode. Inheritable from domain defaults. |
| `error` | Process | `propagate`, `absorb`, `halt` | Error handling policy. Inheritable from domain defaults. |

These fields are irrelevant for embedded and reasoning Blocks. If omitted, values are inherited from the domain chain.

### Observe Fields

```yaml
observe:
  log: ./logs.jsonl
  events: [start, complete, error, tool.call]
```

| Field | Required | Description |
|-------|----------|-------------|
| `observe.log` | No | Path to the log file, relative to the block directory. Default: `./logs.jsonl`. |
| `observe.events` | No | Which events the wrapper should log. Default: `[start, complete, error, tool.call]`. |

The `observe` section declares the block's observability contract. The block wrapper reads this and logs the specified events. Any execution environment that implements the Aglet wrapper protocol — the CLI, a Docker container, a WASM host — reads the same declaration and produces the same logs. See the [Runtime Architecture](/spec/runtime) spec for how wrappers use this.

### Reasoning-Specific Fields

| Field | Required | Description |
|-------|----------|-------------|
| `model` | No | LLM model identifier. Cascades: Block > domain defaults > root domain defaults. |
| `provider` | No | Explicit provider override. If omitted, inferred from model name (`claude-*` -> `anthropic`, `gpt-*` -> `openai`). |

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
# provider: groq              # Optional: explicit provider override

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

## Naming Resolution

Block references in `calls` and `tools` use **adaptive qualification** -- the shortest unambiguous name:

- One `Validator` in the entire project: write `Validator`.
- Two `Validator` Blocks in different domains: qualify as `auth/Validator` and `schema/Validator`.
- Collision at the domain level: go up further -- `payments/auth/Validator` vs `api/auth/Validator`.

`aglet validate` checks all names at startup. If a name is ambiguous, it reports which qualification is needed.

## Implementation Contract

Every process Block implementation follows the same interface regardless of language:

1. Read JSON from stdin
2. Transform the data
3. Write JSON to stdout
4. Exit 0 on success, non-zero on failure

**stdout is for data, stderr is for diagnostics.** Never mix error messages into the stdout JSON stream. Tooling captures stderr and writes it to `logs.jsonl`.

### Code Structure Convention

The implementation has three labeled sections -- **In**, **Transform**, **Out** -- clearly marked with comments. The transformation logic lives in a separate function named after the Block.

**Go:**

```go
package main

import (
    "encoding/json"
    "os"
)

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
        "authorized":     authorized,
        "transaction_id": generateId(),
    }
}
```

**Python:**

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

## How Calls Work

Blocks never invoke each other directly. The `calls` field is a **declaration of data flow**, not a runtime dependency. It says "my output feeds into these Blocks." Tooling reads these declarations to construct pipe chains, validate schema compatibility, and wire Blocks together.

Composition happens outside the Block -- via shell piping, `aglet pipe`, `aglet serve`, or production infrastructure. The Block's implementation never knows about other Blocks, the graph, or the runtime. It just transforms input to output.

To run a single Block: `aglet run BlockName`. To run a pipeline: `aglet pipe StartBlock` (follows `calls` edges to the terminal Block). To serve Blocks as HTTP endpoints: `aglet serve`.

## prompt.md

`prompt.md` is the implementation file for a reasoning Block. It contains the system prompt -- the instructions, worldview, decision framework, and constraints that guide the LLM's reasoning.

Write `prompt.md` as clear instructions to a reasoning system: what the input represents, what judgment to make, what the output should contain, and what constraints to respect. It should be self-contained -- the LLM receives only the prompt (as system message), the input (as user message), and the output schema (as structured output constraint).

`prompt.md` is version-controlled, diffable, and reviewable. Changes to the prompt are changes to the logic.

### Example prompt.md

```markdown
# Sentiment Analyzer

You are a sentiment analysis system. You receive a text input
and determine its emotional sentiment.

## Classification Rules

Analyze the text and produce:
- **sentiment**: One of "positive", "negative", or "neutral"
- **confidence**: A number between 0 and 1 indicating how confident you are
- **reasoning**: A brief explanation of why you chose this sentiment

## Tools

You have access to a WordCount tool. Before classifying sentiment,
use it to count the words in the input text. Include the word count
in your reasoning.

## Constraints

- Be precise with confidence scores. Don't default to 0.5 unless
  truly ambiguous.
- Mixed sentiment texts should be classified by the dominant tone.
- Sarcasm should be classified by the intended meaning, not the
  surface words.
```
