---
title: Core Concepts
---

# Core Concepts

Aglet has four unit types: Blocks, Surfaces, Components, and Domains. Every unit has an `intent.md`. This page covers each in depth.

## Intent

Every unit in an Aglet project has an `intent.md` file. This is the *why* document — not what the unit does (that's in the YAML), but why it exists and what purpose it serves.

If YAML metadata is a unit's DNA — identity, boundaries, capabilities — then `intent.md` is its voice. It's the document that says: *this is why I'm here, and this is what matters about me*.

Intent scales with scope. A domain's `intent.md` reads like a vision statement — the north star for everything inside it. A Block's `intent.md` is focused and specific — why this particular transformation exists and what problem it solves.

The content carries the distinction, not the filename. Every unit's intent file is called `intent.md`, but a domain intent and a Block intent are very different documents.

Intent files aren't just for humans. An AI agent reads them to understand context and purpose before modifying code, generating tests, or suggesting architecture changes. Intent is what turns a collection of functions into a system that explains itself.

## Domain

A domain is an organizational grouping. It's a directory containing a `domain.yaml` and an `intent.md`.

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

Domains carry configuration that their children inherit:

- **`runners`** maps file extensions to execution commands. A Block with `impl: ./main.py` uses the `.py` runner from the nearest ancestor domain.
- **`providers`** configures LLM API access. Each provider has an `env` field pointing to the API key environment variable, and optional `url` and `format` fields for custom endpoints.
- **`defaults`** sets inheritable values for `execution`, `error`, and `model`.

### Config Inheritance

Domains compose fractally — the same structure works at every scale. A sub-domain can override its parent's configuration:

```
my-project/           # domain.yaml: runners, providers, defaults
  analysis/           # domain.yaml: parent: my-project, overrides model default
    SentimentBlock/   # inherits from analysis, which inherits from my-project
```

Resolution walks up the chain: Block -> nearest domain -> parent domain -> root domain. The first value found wins.

### Detection Rule

A directory is a domain if it contains a `domain.yaml`. A root domain has no `parent` field. Sub-domains declare `parent: <domain-name>`.

## Block

A Block is the fundamental unit of being in Aglet — a stateless, self-describing capsule of logic that contains everything needed for independent existence. JSON in, JSON out. It's a directory containing a `block.yaml`, an `intent.md`, and an implementation.

Think of a Block like a biological cell: locally self-contained, with its own identity and purpose, yet constantly signaling the whole through typed schemas and declared edges.

```yaml
id: b-11111111-2222-3333-4444-555555555555
name: SentimentAnalyzer
description: "Analyzes the sentiment of a text input"
domain: my-project
role: classifier
runtime: reasoning
model: claude-sonnet-4-20250514
prompt: ./prompt.md
tools:
  - WordCount

schema:
  in:
    type: object
    properties:
      text:
        type: string
    required:
      - text
  out:
    type: object
    properties:
      sentiment:
        type: string
        enum: [positive, negative, neutral]
      confidence:
        type: number
        minimum: 0
        maximum: 1
      reasoning:
        type: string
    required:
      - sentiment
      - confidence
      - reasoning
```

Key fields:

- **`id`** -- typed UUID with a `b-` prefix
- **`schema.in` / `schema.out`** -- JSON Schema for the Block's input and output, declared inline
- **`runtime`** -- how the Block executes (see below)
- **`role`** -- semantic label (transformer, classifier, validator, etc.) for discovery
- **`calls`** -- list of other Blocks this Block sends data to (data flow edges)
- **`tools`** -- list of Blocks available as tools during reasoning (reasoning runtime only)

### Runtime: Process

A process Block is a script or binary that reads JSON from stdin and writes JSON to stdout. Any language works.

```yaml
runtime: process
impl: ./main.py
```

The `impl` field points to the entry file. The runner is resolved from `domain.yaml` by file extension. If `impl` is omitted, Aglet looks for any `main.*` file in the directory.

```python
import json, sys

data = json.load(sys.stdin)
result = {"word_count": len(data["text"].split())}
json.dump(result, sys.stdout)
```

Process Blocks are the workhorse. They're simple, testable, and portable.

### Runtime: Embedded

An embedded Block is a pure function that lives inside a Surface. It can't be called externally -- it's an internal building block for frontend logic.

```yaml
runtime: embedded
impl: ./transform.ts
```

Embedded Blocks follow the same schema contract as process Blocks, but they're imported directly by Components rather than executed as subprocesses.

### Runtime: Reasoning

A reasoning Block uses an LLM as its runtime. The implementation is a `prompt.md` file -- natural language instructions that define the Block's behavior.

```yaml
runtime: reasoning
model: claude-sonnet-4-20250514
prompt: ./prompt.md
tools:
  - WordCount
```

The `model` field specifies which LLM to use (inheritable from domain defaults). The `tools` field lists Blocks that the LLM can call during execution -- these become tool definitions in the API call, and the CLI handles the tool-use loop automatically.

The `prompt.md` is the implementation:

```markdown
# Sentiment Analyzer

You are a sentiment analysis system. You receive a text input
and determine its emotional sentiment.

## Classification Rules

- **sentiment**: One of "positive", "negative", or "neutral"
- **confidence**: A number between 0 and 1
- **reasoning**: A brief explanation of your classification
```

The output schema from `block.yaml` is enforced as structured output. The LLM must produce JSON matching the schema -- the protocol guarantees it.

### Calls Edges

The `calls` field declares data flow between Blocks:

```yaml
calls:
  - Enricher
  - Validator
```

These edges are declarations, not runtime dependencies. They tell you (and your agent) how data flows through the system. `aglet pipe` uses them to build and execute pipelines.

Edges are how Blocks signal each other — the connective tissue that turns independent units into a living system. The topology of your application emerges from these declarations, not from a central orchestrator wiring things together.

### Detection Rule

A directory is a Block if it contains a `block.yaml`.

## Surface

A Surface is a stateful frontend. It's a directory containing a `surface.yaml` and an `intent.md`.

```yaml
id: s-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee
name: dashboard
description: "Analytics dashboard"
domain: my-project
framework: next
entry: ./src/app/page.tsx

contract:
  dependencies:
    Sentiment:
      block: SentimentAnalyzer
      intent: "Classify user feedback"
      input:
        type: object
        properties:
          text:
            type: string
      output:
        type: object
        properties:
          sentiment:
            type: string
          confidence:
            type: number
      callers:
        - FeedbackPanel
      trigger: user-action
```

The `contract` section is the bridge between frontend and backend. Each dependency maps a name to a Block (or pipeline), with its own input/output schemas, the Components that call it, and when it fires.

`aglet listen` reads this contract and spins up a local HTTP dev server with `POST /contract/<Name>` endpoints, so your frontend can call Blocks without any custom API wiring.

### Detection Rule

A directory is a Surface if it contains a `surface.yaml`.

## Component

A Component is a stateful unit within a Surface. It's a directory containing a `component.yaml`.

```yaml
id: c-11111111-aaaa-bbbb-cccc-dddddddddddd
name: FeedbackPanel
description: "Displays and submits user feedback"
domain: my-project
role: interactive

consumes:
  - Sentiment
```

The `consumes` field declares which contract dependencies this Component uses. This creates a bidirectional link: the Surface contract lists the Component as a `caller`, and the Component lists the dependency in `consumes`. `aglet validate` checks that these stay in sync.

### Detection Rule

A directory is a Component if it contains a `component.yaml`.

## Typed UUIDs

Every unit has an `id` field using a typed UUID format:

| Prefix | Unit Type |
|--------|-----------|
| `b-`   | Block     |
| `s-`   | Surface   |
| `c-`   | Component |
| `d-`   | Domain    |

The prefix makes it immediately clear what kind of unit you're looking at, whether in YAML files, logs, or analytics. `aglet validate` checks that prefixes match unit types and that IDs are unique across the project.

## Detection Summary

What makes a directory a specific unit type:

| File Present       | Unit Type |
|--------------------|-----------|
| `block.yaml`       | Block     |
| `surface.yaml`     | Surface   |
| `component.yaml`   | Component |
| `domain.yaml`      | Domain    |

A directory can be both a Domain and contain Blocks/Surfaces (the domain is the parent, the children are nested inside it). But a directory can only be one of Block, Surface, or Component.
