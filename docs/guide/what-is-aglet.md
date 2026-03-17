---
title: What is Aglet?
---

# What is Aglet?

Aglet is a protocol for self-describing computation.

A Block is to Aglet what a cell is to life — locally self-contained, yet constantly signaling the whole. Every unit of work carries its own identity, intent, typed schemas, and wiring metadata. This isn't decoration — it's the protocol itself. The metadata is the organism's DNA: it tells you who this unit is, what it does, and what it may touch.

Any infrastructure that can read a `block.yaml`, send JSON matching the input schema, and receive JSON matching the output schema can host an Aglet Block. No SDK. No runtime dependency. Just structured data in, structured data out.

## Two Layers, Always in Sync

An Aglet project has two layers that move together:

**The design layer** is YAML and Markdown. It declares what each unit is (`block.yaml`, `surface.yaml`, `component.yaml`, `domain.yaml`), why it exists (`intent.md`), and how it connects to other units (`calls` edges, `contract` dependencies). This layer is the source of truth for identity, schemas, and data flow.

**The code layer** is the implementation. A Python script, a Go binary, a prompt file for an LLM -- whatever actually does the work. The code layer serves the design layer, not the other way around.

When these two layers drift apart, `aglet validate` catches it. When they're in sync, every unit in your project is discoverable, traceable, and independently deployable.

## The Core Protocol

The protocol is simple:

1. A directory contains a `block.yaml` with a typed UUID, a name, input/output schemas, and a runtime declaration.
2. You send JSON matching the input schema to that Block.
3. You receive JSON matching the output schema back.

That's it. The Block doesn't know or care who called it, what infrastructure it's running on, or whether it's being orchestrated by a CLI, a cloud function, or a cURL command. The metadata makes it self-describing. The schemas make it interoperable. The protocol makes it portable.

## Observable by Default

Aglet doesn't bolt observability on after the fact. Every unit has:

- **Semantic identity** — a typed UUID (`b-` for Blocks, `s-` for Surfaces, `c-` for Components, `d-` for Domains) and a human-readable name
- **Typed schemas** — JSON Schema for inputs and outputs, declared inline in the YAML
- **Intent** — a Markdown document explaining *why* this unit exists
- **Edges** — `calls` fields declaring which Blocks flow into which, `contract` sections declaring what data Surfaces need

Discovery, traceability, and analytics aren't features you build on top. They're natural consequences of the structure.

This has a deeper implication: because every unit is self-describing, an entire ecosystem of tooling — analytics dashboards, compliance checks, dependency visualization, cost analysis — becomes possible *without Aglet itself building any of it*. Aglet provides the substrate. The intelligence layer is an open field.

## Agent-Native

The metadata that makes Aglet observable to humans makes it equally legible to AI agents. An agent can read `block.yaml` to understand what a unit does, `intent.md` to understand why, `calls` edges to understand how units connect, and input/output schemas to understand the data contract. No special tooling required — the project describes itself.

Aglet doesn't build its own agent. Every programmer already has one — Claude Code, Cursor, Copilot, or something else entirely. Aglet's job is to make *your* agent dramatically more effective by giving it a codebase that speaks for itself. Transparency over abstraction. The code breathes back.

## The CLI is a Dev Toolkit

The `aglet` CLI (`aglet run`, `aglet pipe`, `aglet serve`, `aglet validate`) is a development convenience. It's useful. It's not required. Blocks don't depend on it to execute. The CLI reads the same YAML metadata that any other system could read, and dispatches execution the same way any other system could.

Think of it like `go run` — helpful for development, but your Go code doesn't depend on it existing.

## The Taxonomy

Aglet has four unit types:

**Blocks** are stateless, single-responsibility computation. JSON in, JSON out. Each Block is a self-contained capsule of logic — everything needed for independent existence lives in its directory. They come in three runtimes:

- **Process** — a script or binary that reads stdin, writes stdout. Any language.
- **Embedded** — pure functions that live inside Surfaces. Internal building blocks, not externally callable.
- **Reasoning** — an LLM call where `prompt.md` is the implementation and the model is the runtime. Can use other Blocks as tools.

**Surfaces** are stateful frontends. They define a `contract` that maps dependency names to Blocks or pipelines, bridging the frontend/backend boundary with typed schemas.

**Components** are the building blocks of Surfaces. Stateful units that declare which contract dependencies they `consume`.

**Domains** organize everything. They carry config inheritance (runners, providers, defaults) and compose fractally — a domain can contain sub-domains, which inherit and override their parent's configuration. The same structure works at every scale, from a single-developer project to a planetary network of services.
