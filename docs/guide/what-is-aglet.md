---
title: What is Aglet?
---

# What is Aglet?

Most software is opaque. To understand what a function does, you read its code. To understand how functions connect, you trace call graphs. To understand what's actually running in production, you check dashboards that live outside the codebase. To understand *why* something was built, you hope someone wrote a comment.

Aglet makes software that describes itself.

## The Core Idea

Every piece of an Aglet application carries its own identity file — a YAML document that says what the unit is, what it accepts, what it produces, what it connects to, and how it wants to be observed. Every unit also has an `intent.md` explaining *why* it exists. Together with typed schemas and runtime logs, the result is a system where any reader — human or AI — can understand the entire application by reading its files.

This creates two layers of knowledge about every unit:

**The declared layer** — what you designed. Identity, schemas, edges, role, intent. This is authored by you and lives in YAML and markdown files that are version-controlled, diffable, and reviewable.

**The behavioral layer** — what actually happens. Call counts, latency, error rates, warmth, observed dependencies. This is written by the runtime and accumulates in the same YAML files over time.

Together, they form the **Semantic Overlay**: a complete, self-describing picture of every unit in your application. Not just the design intent, but the operational reality.

## What You Build With

An Aglet project has four kinds of units:

**Blocks** are the computation. Each Block reads JSON in, transforms it, and writes JSON out. Blocks come in three runtimes: process (scripts that run as subprocesses), reasoning (LLM prompts that execute via API), and embedded (pure functions inside frontends). Every Block has typed input/output schemas and declares its edges to other Blocks.

**Surfaces** are the frontends. A Surface is an entire deployable application — a web app, dashboard, or mobile client. It declares a **contract**: a typed specification of every backend Block it depends on. The system reads this contract to generate HTTP endpoints automatically.

**Components** are the UI building blocks inside Surfaces. They handle orchestration — when to fetch data, when to update state, when to trigger navigation. Transformation logic always goes in embedded Blocks, not in Components.

**Domains** are the organizational layer. A Domain carries configuration (language runners, LLM providers, defaults) and groups related units. Domains nest fractally — a sub-domain inherits from its parent. The root domain is the project itself.

## What Makes It Different

**Self-describing.** A Block's `block.yaml` contains everything needed to understand, execute, and integrate the Block — without reading its implementation. Identity, schemas, edges, observability contract, and behavioral memory, all in one file.

**Observable by default.** Every Block execution is wrapped with automatic logging: start events, completion events, error events, stderr capture, version tracking, and behavioral memory updates. The observe contract lets each Block declare which events it cares about. Surfaces get their own logs from both server-side contract calls and client-side SDK events.

**Agent-native.** The same metadata that makes the protocol work for machines makes it work for AI agents. An agent reading `block.yaml` sees the schemas, the intent, the edges, and the behavioral profile. It knows what changed, what's hot, what's cold, and what depends on what — before reading a single line of code.

**Language-agnostic.** Process Blocks speak stdin/stdout — any language with a runner works. Reasoning Blocks speak natural language — the prompt is the implementation. The contract is JSON Schema. The only language requirement is that you can read JSON and write JSON.

## How It All Connects

The pieces build on each other:

1. **You scaffold units** with `aglet new` — each born complete with identity, intent, and observability.
2. **You run Blocks** and the wrapper observes — logging events, tracking code changes, updating behavioral memory.
3. **You connect Blocks** via `calls` edges — the wrapper handles pipeline propagation automatically.
4. **You serve Surfaces** via the domain listener — one entry point for frontend and backend, with SDK config injection.
5. **The AML accumulates** — behavioral memory grows with every run, building the Semantic Overlay.
6. **You validate** — structural checks catch drift between design and reality.
7. **Agents read it all** — YAML + intent + behavioral memory = a codebase that explains itself.

For the full walkthrough of how these pieces work together, see [How It Works](/guide/how-it-works).

## Next Steps

- **[Getting Started](/guide/getting-started)** — Build your first Aglet project in 10 minutes.
- **[How It Works](/guide/how-it-works)** — The full picture of execution, observation, and connection.
- **[Agent Setup](/guide/agent-setup)** — Set up your AI agent to work with Aglet metadata.
