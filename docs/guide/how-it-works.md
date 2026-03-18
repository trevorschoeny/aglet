---
title: How It Works
---

# How It Works

This page walks through the entire system — what happens when you create, run, connect, observe, and validate an Aglet project. Read it front to back; each section builds on the previous one.

## Creating Units

`aglet init my-app` creates the root domain with three files: a `domain.yaml` (runners, providers, defaults), an `intent.md` (the project's north star), and a `CLAUDE.md` (agent context pointing to the full spec).

From there, `aglet new` scaffolds units. Every unit is born complete — all required files generated in one pass. If any file creation fails, the entire directory is cleaned up.

`aglet new block FetchPage` creates a process Block with a Go implementation following the In/Transform/Out convention, an observe contract, and an intent template. `aglet new block EmailClassifier --runtime reasoning` creates a reasoning Block with a prompt stub and `tool.call` in its observe events. `aglet new surface Dashboard` creates a Surface with React/Vite defaults, an SDK config section, and an empty contract. `aglet new component FeedbackPanel` creates a Component with the SDK already wired — `createAglet`, `mount()`, `unmount()`, `destroy()` — so observability is built in from the first line.

Domain inference is automatic. When you scaffold any unit, the CLI walks up from your current directory looking for the nearest `domain.yaml` and uses its name. You can override with `--domain`, but inference almost always gets it right.

## Running a Block

When you run `aglet run EmailClassifier`, the CLI discovers the Block by scanning the project for a `block.yaml` with that name, locates the root domain for configuration, reads input, and hands everything to the **wrapper**.

### The Wrapper

The wrapper is the heart of the system. It is not the Block — it is the Block's membrane. It handles everything the Block shouldn't think about: version tracking, observability, downstream forwarding, surface logging, and behavioral memory. The Block's implementation stays pure.

This separation is fundamental. The **implementation** (main.py, prompt.md) is self-contained — read input, transform, write output. It can be tested in isolation, moved between projects, or compiled to WASM. The **wrapper** is the Block's network-facing layer — it reads the Block's YAML, handles observability, communicates with other units, and participates in pipelines.

Here is what happens inside the wrapper when a Block runs:

**Version detection.** The wrapper hashes the implementation file and compares it to the last hash in `logs.jsonl`. If the code changed, it emits a `block.updated` event with the new hash and git metadata (commit, author, dirty flag). This tells the AML to reset its measurement window — behavioral data from old code is no longer meaningful.

**Pre-execution logging.** The wrapper logs `block.start` with metadata: which runner will execute the subprocess (process blocks), which model and provider (reasoning blocks), which tools are available. This happens before execution so that if the Block crashes, the start event is already recorded.

**Pre-warming downstream Blocks.** Concurrently, the wrapper reads the `calls` field and resolves all downstream Blocks — locally via filesystem discovery, or remotely via the domain's `peers` table. By the time execution finishes, the downstream wrappers are ready to receive input with zero cold-start delay.

**Execution.** The wrapper calls the executor. For process blocks, `ExecuteProcessBlock` spawns a subprocess, pipes JSON to stdin, captures stdout and stderr separately. For reasoning blocks, `ExecuteReasoningBlock` reads the prompt, resolves the model and provider, makes the LLM API call, handles tool-use loops (each tool call going through its own wrapper), and enforces structured output. The executor is pure — it never touches `logs.jsonl`.

**Post-execution.** The wrapper records duration, logs stderr (always, not just on error), and logs `block.complete` or `block.error` with all metadata. If the Block was called via a Surface contract, it writes a `contract.call` entry to the Surface's `logs.jsonl`. Then it updates behavioral memory — the AML silently observing.

**Forwarding.** If the Block has `calls` edges, the wrapper sends output to each pre-warmed downstream wrapper. For linear pipelines, the chain auto-propagates. For fan-out, all downstream Blocks execute concurrently. Remote Blocks are reached via HTTP POST to the peer domain's listener.

Every execution path — `aglet run`, `aglet pipe`, `aglet listen`, tool calls during reasoning — goes through this same wrapper. One entry point for all block execution with full observability.

### The Observe Contract

Every Block declares which events it wants logged:

```yaml
observe:
  log: ./logs.jsonl
  events: [start, complete, error]
```

Reasoning blocks also include `tool.call`. The wrapper checks this before each logging call — if the Block hasn't opted into an event, the wrapper skips it. Blocks without an observe section log everything (backwards compatible).

This makes observability portable: any execution environment (the CLI, a WASM host, a Docker adapter) reads the same declaration and produces the same logs.

## Pipelines

Blocks connect through `calls` edges — forward data flow declarations. When Block A declares `calls: [BlockB]`, it means "my output feeds into BlockB." The Block's implementation never knows about other Blocks. Composition happens through the wrapper.

`aglet pipe EmailClassifier` triggers the start Block's wrapper, which auto-forwards through `calls` edges. Each Block in the chain gets full observability — start, complete, metadata, behavioral memory updates. The pipeline definition lives in the YAML, not in orchestration code.

`aglet pipe StartBlock EndBlock` does explicit path-finding — BFS through the calls graph to find the shortest chain, then runs each Block in sequence with auto-forwarding disabled to prevent double-execution.

## The Domain Listener

`aglet listen` starts a per-domain HTTP server. It discovers all non-embedded Blocks and exposes them at `POST /block/{BlockName}`, each going through the wrapper.

If the domain contains a Surface, the listener becomes a unified entry point for frontend and backend. It starts the Surface's dev server as a child process, creates a reverse proxy, and intercepts HTML responses to inject `window.__AGLET__` — the SDK configuration from `surface.yaml`. The developer's browser talks to one origin; the listener handles everything.

The listener registers `POST /contract/{DependencyName}` endpoints from the Surface's contract. When a component calls `aglet.call('Sentiment', { text })`, the SDK sends headers identifying the caller and surface. The listener routes this to the Block wrapper, which executes the Block and writes the contract call to the Surface's log.

Client-side events (mount, unmount, custom tracking) are flushed by the SDK to `POST /_aglet/events`, which the listener appends to the Surface's `logs.jsonl`.

The same binary works in dev and production. The only thing that changes is the `peers` table — localhost ports in dev, real URLs in prod.

## Cross-Domain Routing

Domains communicate through `peers` — a routing table in `domain.yaml`:

```yaml
peers:
  payments: "https://payments.myapp.com"
```

When a Block's `calls` reference a domain-qualified name (`payments/PaymentAuth`), the wrapper extracts the domain prefix, looks it up in peers, and forwards via HTTP POST. The remote domain's listener runs the Block through its own wrapper — full observability on both sides.

The routing is explicit, declared in YAML, visible to anyone reading the config. No service discovery, no magic. Each domain knows its neighbors and forwards what it can't handle locally.

## The Adaptive Memory Layer

Every time a Block runs, the AML accumulates knowledge. After each successful execution, the wrapper recomputes behavioral memory from `logs.jsonl` and writes it back to `block.yaml`:

```yaml
behavioral_memory:
  total_calls: 847
  avg_runtime_ms: 24.3
  error_rate: 0.0012
  warmth_score: 0.91
  warmth_level: hot
  last_called: "2026-03-17T21:09:05Z"
  version_since: "2026-03-10T14:22:00Z"
  token_avg: 1240
  observed_callees:
    ParseDate: 423
    ExtractEntities: 847
  observed_callers:
    TestHarness: 42
  last_updated: "2026-03-17T21:09:10Z"
```

This is the **Semantic Overlay**: the declared layer (what you designed) plus the behavioral layer (what actually happens). An AI agent reading `block.yaml` sees both in one file.

The computation is incremental — the AML checkpoints progress and processes only new log entries each run. When the code changes (detected by `block.updated` events), the measurement window resets.

**Warmth** measures operational relevance: 70% recency (how recently the Block was called) and 30% frequency (how many times). Hot Blocks (score >= 0.7) are in active use. Cold Blocks (score < 0.3) may be dormant.

**Observed edges** are mined from `tool.call` log events. When a reasoning Block calls another Block as a tool, the relationship is recorded. Over time, the AML builds a map of actual runtime dependencies — which may differ from declarations.

`aglet stats` surfaces all of this. For a single Block, the full behavioral profile. For a domain, a rollup. For the project, a thermal map showing which parts of the system are alive.

## Surface Observability

Surfaces have their own `logs.jsonl` with events from two sources.

**Server-side**: when a component calls a Block through a contract endpoint, the Block's wrapper writes a `contract.call` entry to the Surface's log — automatic, no SDK setup needed.

**Client-side**: the `@aglet/sdk` provides per-component instances. Each scaffolded component already has the SDK wired in. All instances share a buffer that flushes every 5 minutes and on page unload via `sendBeacon`. The SDK has zero DOM interaction — mount, unmount, and tracking are explicit calls.

```typescript
const aglet = createAglet('FeedbackPanel')
aglet.mount()
const result = await aglet.call('Sentiment', { text })
aglet.track('analysis_complete', { confidence: 0.95 })
aglet.unmount()
aglet.destroy()
```

## Validation

`aglet validate` checks structural integrity: UUIDs, name-folder agreement, intent files, domain references, implementation files, schemas, calls edges, circular dependencies, surface contracts, and component sync. It auto-fixes what it can (missing intents, name mismatches, bidirectional link drift) and reports what needs manual attention.

`aglet validate --deep` generates a judgment-based checklist for an AI agent. The CLI doesn't call an LLM — it produces structured prompts with specific questions, file references, and contextual notes (warmth levels, stub detection). Categories: intent accuracy, schema accuracy, prompt quality, single responsibility, implementation conventions, contract completeness, and logic division.

The divergence check is key: validate compares a reasoning Block's declared `tools` against `observed_callees` from behavioral memory. If the Block uses a tool it never declared, or declared one it never uses, that's drift between design and reality.

## The Full Circle

An Aglet project starts with a domain and a vision. You scaffold Blocks that transform data, Surfaces that present it, Components that orchestrate interactions. Each unit carries its identity, its intent, and its observability contract.

When Blocks run, wrappers observe them. When they chain through pipelines, wrappers propagate. When domain listeners serve HTTP, they inject SDK config and route contract calls. The AML accumulates knowledge about every Block's behavior. Validation catches structural drift. Deep checks generate agent review prompts.

The declared layer is the design. The behavioral layer is the reality. The Semantic Overlay is both, in one file. The codebase doesn't just run code — it accumulates knowledge about itself and makes that knowledge available to anyone who reads it.
