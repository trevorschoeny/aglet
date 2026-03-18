---
title: Agent Setup
---

# Agent Setup

Aglet projects are designed to be legible to AI agents out of the box. The metadata that makes the protocol work for machines also makes it work for your coding assistant. This page covers setup and the three key agent workflows.

## CLAUDE.md

When you run `aglet init`, a `CLAUDE.md` is generated alongside `domain.yaml` and `intent.md`. It contains the project identity, a pointer to the full specification at [trevorschoeny.github.io/aglet](https://trevorschoeny.github.io/aglet/), a quick reference for unit types, and a CLI cheatsheet.

Claude Code reads this automatically at the start of every session. For other agents:

- **Cursor** — copy to `.cursorrules`
- **GitHub Copilot** — copy to `.github/copilot-instructions.md`
- **Any MCP-compatible agent** — reads `CLAUDE.md` directly via file access

The full spec lives at the docs site, not embedded in the file. This keeps `CLAUDE.md` stable across versions.

## What Your Agent Gets

An Aglet project is self-describing. Here's what each file gives your agent:

| File | What it reveals |
|------|----------------|
| `block.yaml` | Identity, schemas (input/output contracts), runtime type, edges to other Blocks, observability contract, behavioral memory |
| `intent.md` | Why this unit exists, design decisions, open questions — context for making good modification decisions |
| `calls` edges | Data flow graph — which Blocks feed into which, where pipelines start and end |
| `behavioral_memory` | Operational reality — warmth, error rate, call frequency, observed dependencies, token usage |
| `surface.yaml` | Frontend contract — which backend Blocks the Surface depends on, which Components call which contracts |
| `component.yaml` | What contract dependencies this Component consumes — bidirectional traceability with the Surface contract |

The behavioral memory section is especially valuable. An agent can see that a Block is hot (actively used, changes carry risk), cold (dormant, safe to refactor), has a high error rate (needs investigation), or uses tools it never declared (design drift). See the [AML specification](/spec/aml) for full details.

## Agent Workflows

### Structural Validation

```bash
aglet validate
```

Run this after making changes. It checks UUIDs, name-folder agreement, intent files, domain references, implementation files, schema presence, calls edges, contract sync, and circular dependencies. It auto-fixes what it can. A clean validate means the project's structural integrity is intact.

### Deep Review

```bash
aglet validate --deep
aglet validate --deep --json          # machine-readable
aglet validate --deep --unit BlockName # single unit
```

Generates a judgment-based checklist for your agent. The CLI doesn't call an LLM — it produces structured prompts with specific questions, file references, and contextual notes (warmth levels, stub detection, untested Blocks). Your agent reads the output and performs the review.

Categories: intent accuracy, schema correctness, prompt quality (reasoning Blocks), single responsibility, implementation conventions, contract completeness, and logic division.

### Behavioral Analysis

```bash
aglet stats BlockName --json
```

Before modifying a high-traffic Block, inspect its behavioral memory. This gives your agent the full operational picture — warmth, error rate, observed dependencies, token usage — so it can calibrate how careful to be.

## The Philosophy

Aglet doesn't build agents. It makes projects legible to any agent.

Most codebases require archaeology — reading implementation code, inferring intent, guessing at data flow. Aglet inverts this: the codebase speaks for itself through structured metadata, typed schemas, and accumulated behavioral knowledge. Your agent already knows how to read YAML and follow references. Aglet gives it the structured data to work with.

For the full picture of how execution, observation, and behavioral memory connect, see [How It Works](/guide/how-it-works).
