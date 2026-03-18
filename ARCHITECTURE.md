# How Aglet Works

Aglet is a protocol for building software that describes itself. Every piece of an Aglet application — every function, every frontend, every grouping of code — carries its own identity file that says what it is, what it accepts, what it produces, and why it exists. The runtime reads these declarations to execute, observe, and connect the pieces automatically.

This document describes the system as it exists today: what happens when you create, run, connect, observe, and validate an Aglet project. It's meant to be read front to back, because the pieces build on each other — the way you scaffold a Block shapes how it gets executed, which shapes how it gets observed, which shapes how agents understand it, which shapes how you scaffold the next one.

---

## The Foundation: Self-Describing Units

An Aglet project is made of four kinds of things. Each one is a directory on disk. Each one carries its own identity file — a YAML document that says everything there is to say about that unit without reading its code.

**Blocks** are the computation. A Block is a directory with a `block.yaml` that declares its name, its typed UUID (prefixed with `b-`), its domain membership, its role (transformer, classifier, verifier, gateway), its runtime (process, embedded, or reasoning), its input and output schemas (JSON Schema in YAML syntax), its edges to other Blocks (`calls` for data flow, `tools` for reasoning-time invocations), its observability contract, and — once it's been running — its behavioral memory. A Block reads JSON in, transforms it, and writes JSON out. That's its entire contract with the outside world.

Blocks come in three runtimes, and this is not a minor detail — it's the fundamental design decision for each unit of logic.

**Process blocks** are scripts. They run as subprocesses: the system pipes JSON to stdin, the script does its work, and writes JSON to stdout. Any language works — Go, Python, TypeScript — because the runner is configured in the domain. The implementation follows a strict In/Transform/Out convention: read input, call a named transformation function, write output. Stderr is for diagnostics. Exit 0 means success.

**Reasoning blocks** are LLM prompts. The implementation isn't code — it's a `prompt.md` file containing natural language instructions. The system reads the prompt, resolves the model and provider from the domain configuration, makes the API call with the input as a user message, and enforces the output schema as structured output. If the Block declares `tools`, those tool Blocks become callable during reasoning — the LLM can decide to invoke them mid-thought, and the system handles the tool-use loop automatically, each tool call going through its own wrapper with full observability.

**Embedded blocks** are pure functions that live inside Surfaces. They follow the same schema contract as process blocks (typed input, typed output), but they're imported directly by Components rather than executed as subprocesses. They can't be called externally — the CLI rejects attempts to run them. They exist for frontend transformation logic that shouldn't live in Components.

Every Block also has an `intent.md`. This is not documentation in the traditional sense. It's the *why* document — not what the Block does (that's in the YAML), but why it exists, what design decisions were made, what alternatives were considered, and what questions remain open. An AI agent reads intent before modifying code. A human reads intent before deciding whether to refactor or delete.

**Surfaces** are the frontends. A Surface is an entire deployable application — a web app, a dashboard, a mobile client. Its `surface.yaml` declares its identity (typed UUID with `s-` prefix), its framework and bundler (React/Vite by default), its dev server configuration, its SDK settings, and most importantly its **contract**: a typed specification of every backend Block the Surface depends on.

The contract is the bridge between two worlds. Each entry maps a dependency name to a Block (or pipeline), declares which Components call it, and specifies the input/output schemas. The system reads this contract to generate HTTP endpoints during development: each dependency becomes `POST /contract/{DependencyName}`. The Surface makes standard HTTP requests — it never knows whether the dev server or production infrastructure is answering. The contract is also the primary traceability tool: when someone modifies a backend Block, they trace forward through contracts to know which Surfaces are affected. When a frontend developer needs new data, they add a contract entry, and the backend team knows exactly what to build.

Surfaces also declare events — outbound signals that Components emit. These have their own schemas and emitter lists, creating another layer of typed traceability.

**Components** are the UI building blocks inside Surfaces. Each has a `component.yaml` with a typed UUID (`c-` prefix), a role (page, layout, widget, form, list, card, modal, nav, input), and a `consumes` list declaring which contract dependencies it uses. This creates bidirectional traceability: the Surface contract lists which Components call each dependency (via `callers`), and the Component lists which contract entries it uses (via `consumes`). If they disagree, `aglet validate` catches it and auto-fixes.

Components handle orchestration logic — deciding *when* things happen. They never handle transformation logic — computing *what* things are. That distinction is fundamental. When a Component needs data transformed, it calls an embedded Block. The Component orchestrates; the Block computes. The boundary between them is the function call.

**Domains** are the organizational layer. A Domain is a directory with a `domain.yaml` that carries configuration: `runners` mapping file extensions to execution commands (`.py` → `python3`, `.go` → `go run`), `providers` configuring LLM API access (Anthropic, OpenAI, Groq, local models), and `defaults` for execution mode, error handling, and model selection. Domains can declare `listen: true` to serve as an HTTP listener, and `peers` to route requests to other domains.

Domains nest fractally. A sub-domain declares `parent: <domain-name>` and inherits everything from its parent, overriding what it needs. Configuration resolution walks up the chain: Block → nearest domain → parent domain → root domain. The first value found wins. The root domain is the project itself — there is no separate "project" concept. Because the root is just a domain, any Aglet project can be absorbed into a larger one by giving its root `domain.yaml` a `parent` field.

Every unit gets a typed UUID and an `intent.md`. The typed prefix (`b-`, `s-`, `c-`, `d-`) makes it immediately clear what kind of unit you're looking at, whether in YAML files, logs, or analytics. The ID is generated once via `crypto/rand` and never changes. The intent is a living document that evolves with the unit.

---

## Creating a Project

`aglet init my-app` creates the root domain. This generates three files: a `domain.yaml` with runners for Go, TypeScript, and Python, commented-out provider examples, and execution defaults; an `intent.md` template with sections for the project's purpose, sacred constraints, and intended audience; and a `CLAUDE.md` that gives AI agents a pointer to the full Aglet specification at the docs site, a quick reference for unit types, and a CLI cheatsheet.

The CLAUDE.md is intentionally lean — the full spec lives at the docs URL, not embedded in the file. This means the file is stable across versions. Claude Code reads it automatically at the start of every session; other agents (Cursor, Copilot) can use their own context file formats with the same content.

From there, `aglet new` scaffolds units. The scaffolding is comprehensive — every unit is born complete, with all required files generated in one pass. If any file creation fails, the entire directory is cleaned up to avoid leaving broken state.

`aglet new block FetchPage` creates a process Block with a Go implementation stub following the In/Transform/Out convention. `aglet new block EmailClassifier --runtime reasoning --model claude-sonnet-4-20250514` creates a reasoning Block with a prompt template, model configuration, and `tool.call` in its observe events. `aglet new block StripSignature --runtime embedded --lang ts` creates an embedded Block with a pure TypeScript function export. Each variant gets the right default role (transformer for process/embedded, classifier for reasoning), the right implementation file, and an observe contract appropriate to its runtime.

`aglet new surface Dashboard` creates a Surface with React/Vite defaults, a dev server configuration (`npm run dev` on port 5173), an SDK section with a 5-minute flush interval, and an empty contract ready to be filled.

`aglet new component FeedbackPanel` creates a Component with a `component.yaml`, an intent template, and — critically — a `.tsx` file that already has the SDK wired in. The generated code includes `import { createAglet } from '@aglet/sdk'`, a `useEffect` that calls `mount()` on creation and `unmount()` + `destroy()` on cleanup. Observability is built into the component from its first line of existence. The developer never has to think about adding it.

`aglet new domain intelligence` creates a sub-domain with a `domain.yaml` (parent inferred from the nearest ancestor domain), an intent template, and commented-out `listen: true` and `peers:` fields ready for when the domain needs to serve HTTP traffic.

Domain inference is automatic throughout. When you scaffold a Block, Surface, or Component, the CLI walks up from your current directory looking for the nearest `domain.yaml` and uses its name as the domain field. You can override with `--domain`, but the inference almost always gets it right.

---

## Running a Block

When you run `aglet run EmailClassifier`, the CLI discovers the Block by walking the project tree, scanning every `block.yaml` until it finds one with `name: EmailClassifier`. If the name is ambiguous (multiple Blocks with the same name in different domains), the CLI reports which domain-qualified name to use. Once found, it locates the root domain (walking up from the Block's directory to find the first `domain.yaml` without a `parent` field), reads input from a file argument, stdin pipe, or defaults to `{}`, and hands everything to the wrapper.

`aglet reason ./EmailClassifier` is the direct path — it skips discovery, parses the `block.yaml` from the given directory, verifies it's a reasoning Block, and goes straight to the wrapper.

### The Wrapper: A Block's Membrane

The wrapper is the heart of the system. It is not the Block — it is the Block's membrane. The wrapper handles everything the Block shouldn't have to think about: version tracking, observability, downstream forwarding, surface logging, and behavioral memory. The Block's implementation stays pure — read input, transform, write output. The wrapper provides the context.

This separation is a key design principle. The **implementation** is self-contained. It can be tested in isolation, moved between projects, compiled to WASM. It has no dependencies on Aglet infrastructure. The **wrapper** is the Block's network-facing layer. It reads the Block's YAML, handles observability, communicates with other Blocks' wrappers, writes to logs, and participates in pipelines. It interacts with other units — writing to Surface logs, pre-warming downstream Blocks, forwarding output along declared edges. The implementation doesn't know the wrapper exists. The wrapper knows everything the Block has declared about itself.

Every execution path in the system — `aglet run`, `aglet pipe`, `aglet listen`, `aglet reason`, tool calls during reasoning — converges on this same wrapper function: `WrapBlock`. There is one entry point for running a Block with full observability. Here is what happens inside it, step by step.

**Step 1: Version change detection.** The wrapper hashes the implementation file (SHA-256) and compares it to the last hash recorded in `logs.jsonl`. If the file has changed, it queries git for metadata — the short commit hash, commit message, author, timestamp, and whether there are uncommitted changes (the dirty flag). If the hash is different, it emits a `block.updated` event with the new hash and git metadata. This is how the AML knows when to reset its measurement window — behavioral data from the previous version of the code is no longer meaningful for the current version. The version tracking is automatic and silent.

**Step 2: Pre-execution metadata.** The wrapper gathers runtime-specific information for the start log: which runner will execute the subprocess (for process blocks), which LLM model and provider will handle the reasoning (for reasoning blocks), which tools are available. This is a lightweight probe — no execution happens yet.

**Step 3: Log block.start.** The wrapper writes a `block.start` event to `logs.jsonl` with the version information and pre-execution metadata. This happens before execution so that if the Block crashes, hangs, or never completes, the start event is already recorded. But only if the Block's observe contract includes `start` in its events list — if not, this step is skipped silently.

**Step 3.5: Pre-warm downstream Blocks.** Concurrently with the metadata gathering, the wrapper reads the Block's `calls` field and begins resolving all downstream Blocks. For each name in `calls`, it launches a goroutine that discovers the Block — locally via filesystem scanning, or remotely via the domain's `peers` table. If the name is domain-qualified (`payments/PaymentAuth`), the wrapper extracts the domain prefix, looks it up in peers, and prepares an HTTP endpoint. By the time the current Block finishes executing, the downstream wrappers are already resolved and ready to receive input immediately — zero cold-start delay on the handoff.

**Step 4: Execute.** The wrapper calls the appropriate executor based on the Block's runtime.

For **process blocks**, `ExecuteProcessBlock` resolves the implementation file from `block.yaml`, looks up the runner from the domain's `runners` map by file extension, builds the command, pipes the input to stdin, and captures stdout and stderr into separate buffers. Stderr is captured regardless of exit code — even on success, diagnostic output is preserved. The executor returns an `ExecutionResult` containing the stdout output, all stderr, any error, and metadata (runner command, implementation file, exit code on failure). The executor is pure: it never touches `logs.jsonl`.

For **reasoning blocks**, `ExecuteReasoningBlock` reads the prompt from `prompt.md`, resolves the model (Block config → domain defaults → root domain defaults), resolves the provider (inferred from model name prefix — `claude-*` → Anthropic, `gpt-*` → OpenAI — or explicit in config), reads the API key from the environment variable declared in the provider config, and builds the API request. The input becomes the user message. The prompt becomes the system message. The output schema from `block.yaml` is enforced as structured output.

If the Block declares `tools`, those Blocks are resolved, verified to be process or reasoning runtime (never embedded), and their input schemas become tool definitions in the API call. The executor then enters the tool-use loop: send the request, check if the LLM wants to call a tool, execute the tool Block (through its own wrapper — full observability, all the way down), feed the result back to the LLM, and repeat. The loop runs up to 20 iterations. Tool calls and results are logged as `tool.call` and `tool.result` events inside the executor — these are execution-level events, not wrapper-level.

The system supports two LLM protocols: Anthropic's Messages API and OpenAI's Chat Completions API (which also covers compatible providers like Groq and local models). Each has its own request/response format, tool definition schema, and structured output mechanism. The provider resolution handles the differences transparently.

The executor returns an `ExecutionResult` with the structured output, any error, and metadata (model, provider, input/output tokens, number of tool loops).

**Step 5: Record duration.** Milliseconds since the start timestamp.

**Step 6: Log stderr.** If the executor captured any stderr output, the wrapper emits a `stderr` event and prints it to the CLI's stderr so the developer sees it. This happens on success too — if your Python script prints a warning, it shows up in the logs and on screen.

**Step 7: Merge metadata.** The executor's metadata (tokens, model, runner, etc.) gets merged with the duration into a single metadata object for the completion log.

**Step 8: Log completion or error.** If the Block succeeded, the wrapper writes `block.complete` with duration, output size, and all metadata — but only if the observe contract includes `complete`. If the Block failed, it writes `block.error` with the error message and metadata — but only if the observe contract includes `error`. If the Block was called through a Surface contract endpoint, the wrapper also writes a `contract.call` entry to the Surface's `logs.jsonl`, regardless of success or failure.

**Step 9: Update behavioral memory.** After a successful run, the wrapper recomputes the Block's behavioral memory from its log entries and writes the result back to `block.yaml`. This is the AML passively observing. The cross-block caller scan (which is expensive — O(n) across all Blocks) is skipped in this auto-update path; it only runs during explicit `aglet stats` calls.

**Step 10: Forward to downstream Blocks.** If the Block has `calls` edges and forwarding is enabled, the wrapper sends the output to each pre-warmed downstream wrapper. For a single downstream Block (linear pipeline), it executes and returns the final output — enabling chains to auto-propagate through the entire pipeline. For multiple downstream Blocks (fan-out), all execute concurrently; the current Block's output is returned to the caller. Remote Blocks are forwarded via HTTP POST to the peer domain's listener endpoint. Local Blocks are forwarded via recursive `WrapBlock` calls — full observability at every hop.

---

## The Observe Contract

Every Block scaffolded by `aglet new` includes an `observe:` section in its `block.yaml`:

```yaml
observe:
  log: ./logs.jsonl
  events: [start, complete, error]
```

Reasoning blocks get `[start, complete, error, tool.call]`. The wrapper reads this declaration and checks it before each logging call via `shouldLog(block, eventName)`. If the Block hasn't opted into an event, the wrapper skips it silently. Blocks without an observe section log everything — the default is backwards compatible.

This is the Block's observability contract. It declares how it wants to be observed. The contract is portable: any execution environment that implements the Aglet wrapper protocol — the CLI today, a WASM host tomorrow, a Docker adapter, a serverless adapter — reads the same declaration and produces the same logs. The observability specification lives with the Block, not in the tool that runs it.

---

## Pipelines

Blocks connect through `calls` edges — forward data flow declarations in `block.yaml`. When Block A declares `calls: [BlockB]`, it means "my output feeds into BlockB." This is a design-time declaration, not a runtime dependency. The Block's implementation never knows about other Blocks. Composition happens outside, via the wrapper.

`aglet pipe EmailClassifier` triggers the start Block's wrapper, which auto-forwards through `calls` edges. Block A runs, its wrapper forwards to Block B, Block B's wrapper forwards to Block C, and so on until a terminal Block (one with no `calls`) is reached. The final output is returned. Each Block in the chain gets full observability — start, complete, metadata, behavioral memory updates.

`aglet pipe StartBlock EndBlock` does explicit path-finding. The CLI builds a graph of all Blocks and their `calls` edges, runs BFS to find the shortest path from start to end, and executes that specific sequence. In this mode, the wrapper's auto-forwarding is disabled (via `WrapBlockOptions.ForwardCalls = false`) to prevent double-execution — the pipeline runner manages the sequence.

The pipeline can also detect branches: if a Block has multiple `calls` targets, `FindPipelineFrom` reports the ambiguity. Fan-out pipelines are supported through auto-forwarding (where the wrapper sends output to all targets concurrently), but explicit path-finding requires a linear chain.

---

## The Domain Listener

`aglet listen` starts a per-domain HTTP server that serves as the domain's entry point. It discovers all non-embedded Blocks in the domain and exposes them at `POST /block/{BlockName}`. Each request deserializes the JSON body, finds the Block, and goes through `WrapBlock` with full observability.

If the domain contains a Surface (a directory with `surface.yaml`), the listener becomes a unified entry point for both the frontend and the backend.

First, it reads the Surface's `sdk:` section and builds a JavaScript config script: `window.__AGLET__ = {surface: "Dashboard", flushInterval: 300, ...}`. Then it starts the Surface's dev server as a child process (from the `dev.command` field in `surface.yaml` — typically `npm run dev`), waits for it to become ready by polling the dev port, and creates a reverse proxy.

When the browser requests HTML, JS, or CSS, the listener proxies the request to the dev server. But for HTML responses specifically, the listener intercepts the response body and injects the SDK config script before the closing `</head>` tag. This is how the SDK auto-configures: the developer never passes options manually — they come from `surface.yaml`, through the listener, into the HTML, into `window.__AGLET__`, into the SDK.

The listener registers `POST /contract/{DependencyName}` endpoints from the Surface's contract. When a component calls `aglet.call('Sentiment', { text })`, the SDK sends a POST with `X-Aglet-Caller: FeedbackPanel` and `X-Aglet-Surface: Dashboard` headers. The listener extracts these, resolves the Surface directory, builds a `SurfaceCallContext`, and passes it to `WrapBlockWithOptions`. The wrapper executes the Block and writes the `contract.call` entry to the Surface's `logs.jsonl` — the wrapper doing its network-facing job of communicating with other units.

The listener also registers `POST /_aglet/events` for receiving client-side interaction events. The SDK buffers mount, unmount, and custom tracking events in the browser and flushes them to this endpoint every 5 minutes (or on page unload via `sendBeacon`). The listener parses the batch and appends each event to the Surface's `logs.jsonl`.

A `/health` endpoint returns a simple status check.

The listener replaces the old `aglet serve` (which operated at the project level, serving all Blocks and all Surfaces through a single server). The per-domain model maps to how distributed systems actually deploy: each domain is an autonomous unit with its own listener, its own Blocks, and its own routing table. In development, you start one listener. In production, you deploy one per domain. The binary is the same. The behavior is the same. The logging is the same. The only thing that changes is the `peers` table: localhost ports in dev, real URLs in production.

`aglet serve` still works but prints a deprecation warning pointing to `aglet listen`.

---

## Cross-Domain Routing

Domains communicate through declared `peers` — a routing table in `domain.yaml` that maps domain names to URLs:

```yaml
peers:
  payments: "https://payments.myapp.com"
  auth: "http://localhost:8081"
```

When Block A in domain `intelligence` has `calls: [payments/PaymentAuth]`, the wrapper resolves this during pre-warming. It tries local discovery first — scanning the filesystem for a Block named `PaymentAuth`. When that fails (it's in a different domain), the wrapper extracts the domain prefix `payments`, looks it up in `peers`, and gets the URL. The pre-warmed result is marked as remote with the peer URL.

When it's time to forward the output, the wrapper makes an HTTP POST to `payments.myapp.com/block/PaymentAuth` with the output as the body. The remote domain's listener receives the request, finds the Block, and runs it through its own wrapper — full observability on both sides. The result flows back through the HTTP response.

The routing is explicit, declared, and visible. There is no service discovery, no magic DNS, no central registry. The routing table is in the YAML. Anyone reading `domain.yaml` can see exactly which domains this one talks to and where they live. Like internet routers, each domain knows its immediate neighbors and forwards what it can't handle locally. The global topology emerges from these local declarations.

---

## The Adaptive Memory Layer

The AML is what makes Aglet more than a build tool. Every time a Block runs, the system learns something about it. The AML accumulates that knowledge over time, distills it into a behavioral profile, and writes it back into the same file that carries the Block's declared identity.

This creates the **Semantic Overlay**: the declared layer (what you designed — schemas, edges, role, intent) plus the behavioral layer (what actually happens — call counts, latency, error rates, warmth, observed dependencies). Together they tell the full story of each Block. An AI agent reading `block.yaml` sees both the design intent and the operational reality in one file.

### What Gets Tracked

After each successful execution, the wrapper calls `computeBehavioralMemory` which reads the Block's `logs.jsonl` and computes:

- **total_calls** — how many times the Block has been invoked since the current code version
- **avg_runtime_ms** — mean execution time across all completed calls
- **error_rate** — fraction of calls that ended in error (0.0 = perfect, 1.0 = total failure)
- **warmth_score** — a 0.0–1.0 composite of recency and frequency
- **warmth_level** — hot (≥ 0.7), warm (≥ 0.3), or cold (< 0.3)
- **last_called** — ISO 8601 timestamp of the most recent invocation
- **version_since** — when the current measurement window started (resets on code change)
- **token_avg** — average LLM tokens per call (reasoning blocks only)
- **observed_callees** — map of tool Block names → call counts (mined from `tool.call` events)
- **observed_callers** — map of Block names → times they called this Block as a tool

### Incremental Accumulation

The AML doesn't recount logs from scratch on every run. It uses a checkpoint-and-delta model. It reads the existing `behavioral_memory` from `block.yaml` (the last checkpoint), scans `logs.jsonl` for the most recent `block.updated` event, and determines the window.

If the Block's code has changed since the last checkpoint (the `block.updated` event is newer than `last_updated`), the measurement window resets. All counters start from zero at the point of the code change. Behavioral data from the old version is discarded — it's no longer meaningful.

If no code has changed, the AML enters incremental mode. It seeds base counters from the existing behavioral memory (total_calls, accumulated durations, error counts) and processes only log entries newer than `last_updated`. A Block with millions of historical entries and ten new runs since the last stats write processes ten entries, not millions.

The base counters use an approximation: `baseDurationCount ≈ totalCalls` and `baseTokenCount ≈ totalCalls`. This works because virtually all `block.complete` events include duration data, and for non-reasoning blocks `token_avg = 0` so the math resolves cleanly.

### Warmth

Warmth measures operational relevance — not just how often a Block runs, but whether it's still actively being used. The formula:

```
warmth_score = (recency × 0.7) + (frequency × 0.3)
```

Recency decays from the last call: within an hour → 1.0, within a day → ~0.9, within a week → ~0.7, within a month → ~0.4, within a year → ~0.1, never called → 0.0. Frequency is normalized against a baseline, capped at 1.0.

A Block that ran a million times two years ago and hasn't run since is cold. A Block that ran once yesterday is warm. A Block that runs hundreds of times daily is hot. Warmth is recalculated on every stats write — it's a snapshot of the Block's health at a point in time.

### Observed Edges

The behavioral layer surfaces the Block's actual runtime dependency graph, independent of what was declared.

**Observed callees** are mined from `tool.call` log events in the Block's own `logs.jsonl`. Each entry records which tool Block was called during reasoning. The AML counts these per-window and stores `BlockName → count`. Only reasoning Blocks produce these events (process Blocks don't call tools).

**Observed callers** require a cross-block scan. The AML reads every other Block's `logs.jsonl` and finds `tool.call` events where the tool field matches this Block's name. This tells you which Blocks are actually depending on this one at runtime. The cross-scan is expensive (O(n) across all Blocks), so it's only done during explicit `aglet stats` calls, not on the auto-update after each run.

### Stats Command

`aglet stats EmailClassifier` shows the full behavioral profile for a single Block: warmth, total calls, average runtime, error rate, last called, token usage, observed callees and callers.

`aglet stats --domain intelligence` computes a domain-level rollup: aggregate runtime, error rate, warmth distribution, hottest and coldest Blocks. This is computed on-the-fly, not stored.

`aglet stats --project` produces a thermal map of the entire project — every Block sorted by warmth score descending. At a glance, you see which parts of the system are alive and which are dormant.

`aglet stats EmailClassifier --write` writes the computed behavioral memory back to `block.yaml`. `--json` outputs machine-readable JSON for agent consumption.

---

## Surface Observability

Surfaces have their own `logs.jsonl` in the Surface directory. This file is the Surface's behavioral record, and events flow into it from two sources that correspond to the two halves of a Surface's life: server-side and client-side.

### Server-Side: Contract Call Tracking

When a component calls a Block through a contract endpoint, the Block's wrapper writes a `contract.call` entry to the Surface's `logs.jsonl`. The wrapper can do this because it receives a `SurfaceCallContext` from the domain listener — containing the Surface directory, Surface name, caller Component name, and contract dependency name. All of this comes from the HTTP headers (`X-Aglet-Caller`, `X-Aglet-Surface`) that the SDK adds to every contract call.

The log entry records which contract was called, which Block handled it, which Component triggered it, how long it took, and whether it succeeded. This is automatic — no SDK configuration needed for contract calls. The Block wrapper handles it as part of its network-facing duties.

### Client-Side: The @aglet/sdk

The `@aglet/sdk` is a TypeScript package that provides per-component instances with three capabilities: lifecycle tracking (`mount`/`unmount`), contract calls (`call`), and custom event tracking (`track`).

When `aglet new component FeedbackPanel` scaffolds a component, the generated `.tsx` file already includes:

```typescript
import { useEffect } from "react";
import { createAglet } from "@aglet/sdk";

export function FeedbackPanel({}: FeedbackPanelProps) {
  useEffect(() => {
    const aglet = createAglet("FeedbackPanel");
    aglet.mount();
    return () => {
      aglet.unmount();
      aglet.destroy();
    };
  }, []);
}
```

`createAglet` reads `window.__AGLET__` (injected by the domain listener) for surface name and flush interval. If the injected config isn't present (production without a listener, or a different setup), defaults apply and the flush silently no-ops.

All instances share a single event buffer and a single flush timer. Creating 50 instances (one per component) is lightweight — each is just a small object referencing the shared buffer. The flush fires every 5 minutes (configurable in `surface.yaml`) and on `beforeunload` via `sendBeacon` (which the browser guarantees to deliver even if the page is closing). Events go to `POST /_aglet/events` on the domain listener, which appends them to the Surface's `logs.jsonl`.

The SDK has zero DOM interaction. No click listeners, no mutation observers, no attribute scanning. Mount and unmount are explicit calls. Custom tracking is explicit. This is intentional — the developer controls exactly what gets logged. The SDK is a thin observability layer, not an analytics framework.

`aglet.call('Sentiment', { text })` makes a standard `POST /contract/Sentiment` with the caller and surface headers. The server handles the logging. The SDK is just adding two headers to the fetch call the developer was already going to make. If a developer uses raw `fetch()` instead of `aglet.call()`, the Block still executes fine — they just lose the component attribution in the Surface log.

---

## Validation

`aglet validate` performs deterministic structural checks on the project and auto-fixes what it can. It starts by discovering every unit in the project — walking the filesystem, parsing YAML files, building a complete inventory of Blocks, Surfaces, Components, and Domains.

Then it runs checks, accumulating all errors before reporting:

**UUID checks**: every ID has the correct prefix for its type, follows UUID v4 format, and is unique across the project. A Block with an `s-` prefix or a duplicate ID gets flagged.

**Name-folder agreement**: the `name` field in each YAML must match the directory name. If they disagree, validate auto-fixes the YAML to match the folder.

**Intent files**: every unit must have an `intent.md`. If one is missing, validate creates a stub with the unit name as a heading.

**Domain references**: every `domain` field must reference a domain that actually exists in the project.

**Block files**: process and embedded Blocks must have an implementation file. Process Blocks must have schemas. Runtimes must be valid.

**Reasoning blocks**: must have a model set (or inheritable from domain defaults). Must have a prompt file. Tools must reference existing, executable Blocks (not embedded ones). Providers must be configured.

**Calls edges**: every name in `calls` must reference a real Block. Additionally, validate checks for divergence between declared tools and observed behavior — if `observed_callees` from behavioral memory shows a tool that isn't in `tools`, that's an undeclared runtime dependency. If a tool is in `tools` but never appears in `observed_callees` after 20+ total calls, that's a dead declaration.

**Schema compatibility**: basic structural validation of schemas.

**Circular dependencies**: DFS detects cycles in the calls graph.

**Surfaces**: entry file must exist. No nested Surfaces (a Surface can't contain another Surface). Contract dependencies must reference real Blocks or pipelines.

**Components**: must exist within a parent Surface. `consumes` entries must correspond to contract dependencies in the parent Surface. The bidirectional link (contract `callers` ↔ component `consumes`) is checked, and mismatches are auto-fixed.

**Domains**: `parent` field must reference an existing domain. If invalid, validate infers the parent from filesystem nesting and auto-fixes.

After all checks run, validate reports what it fixed automatically, what still needs manual attention, and a summary count. The auto-fix philosophy is: if the correct answer is unambiguous, fix it; if it's ambiguous, report it.

### Deep Validation

`aglet validate --deep` generates a judgment-based checklist for an AI agent. The CLI doesn't call an LLM — it produces structured prompts that tell the agent exactly what to evaluate.

Each check is a `DeepCheck` with an ID, a category, the unit under review, a prompt (a specific question the agent should answer by reading the files), file paths to read, and contextual notes. The notes include warmth level from behavioral memory ("this block is hot — treat changes carefully"), stub detection ("intent.md appears to be a stub — no real content yet"), and testing status ("total_calls: 0 — this block has never been executed").

The categories cover seven dimensions:

- **Intent accuracy**: does `intent.md` match what the implementation actually does?
- **Schema accuracy**: do `schema.in` and `schema.out` match what the code reads and writes?
- **Prompt quality**: is the reasoning prompt comprehensive? Does it handle edge cases?
- **Single responsibility**: does this Block do exactly one thing?
- **Implementation convention**: does the code follow the In/Transform/Out pattern?
- **Contract completeness**: does the Surface contract cover all external data needs?
- **Logic division**: is transformation logic in embedded Blocks, not inline in Components?

Output can be human-readable markdown (a categorized checklist with preamble) or JSON (for programmatic agent consumption). `--unit EmailClassifier` filters to a specific unit.

The deep check is designed to be run, handed to an agent, and acted on. The CLI generates the questions; the agent provides the judgment. This keeps the CLI dependency-free while enabling genuine architectural analysis through whatever LLM the developer already uses.

---

## The Full Circle

Here is how all the pieces connect.

You start with `aglet init`. The project is born as a root domain with a vision, configuration, and an agent context file. You scaffold Blocks with `aglet new block`, each declared with schemas, edges, and an observe contract. You scaffold a Surface with `aglet new surface`, declaring its contract to the Block world. You scaffold Components with `aglet new component`, each pre-wired with the SDK.

When you run a Block, the wrapper observes it. It detects code changes, logs start and complete events, captures stderr, and updates behavioral memory. The Block's `logs.jsonl` grows with every invocation. The AML distills this into a behavioral profile — warmth, error rate, runtime distribution, observed dependencies — and writes it back to `block.yaml`.

When you pipe Blocks together, the wrappers chain through declared `calls` edges. Each Block in the pipeline gets full observability. Pre-warming eliminates cold-start delays between links. The topology of the pipeline is visible in the YAML — not hidden in orchestration code.

When you start the domain listener, it serves the frontend through a reverse proxy, injecting SDK config into the HTML. The Surface's dev server runs as a child process behind the listener. Every contract call from the browser flows through the listener to a Block's wrapper, which executes the Block and writes to both the Block's log (block-level observability) and the Surface's log (surface-level observability). Client-side events from the SDK flush to the listener and accumulate in the Surface's log too.

When you run `aglet stats`, the AML reads the logs and shows you what's really happening. Which Blocks are hot (actively used, changes carry risk). Which are cold (dormant, safe to refactor). Which tools a reasoning Block actually uses versus what it declared. Which components call which contracts. The thermal map of the project.

When you run `aglet validate`, the system checks its own structural integrity. UUIDs are correct. Names match folders. Intent files exist. Schemas are present. Calls reference real Blocks. Contracts reference real pipelines. Components and contracts agree on who calls what. No circular dependencies. And with `--deep`, it generates a review checklist that an AI agent can act on — using the warmth data, the stub detection, and the behavioral memory to prioritize what matters.

When domains need to communicate, `peers` provides the routing table. Each domain is autonomous — its own listener, its own Blocks, its own routing. The system scales by adding domains, not by growing a monolith. Cross-domain calls go through the same wrapper, with the same observability, just routed over HTTP instead of function calls.

The declared layer — YAML, schemas, edges, intent — is the design. The behavioral layer — logs, warmth, observed edges, error rates — is the reality. The Semantic Overlay is both, in one file, readable by any agent. The system doesn't just run code. It accumulates knowledge about its own behavior and makes that knowledge available to anyone who reads its files.

Nothing is hidden. Nothing requires tribal knowledge. Nothing exists only in someone's head. The codebase is a book that any agent — human or artificial — can read from front to back, understanding not just what the system was designed to do, but what it actually does in practice.
