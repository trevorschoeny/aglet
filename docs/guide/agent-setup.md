---
title: Agent Setup
---

# Agent Setup

Aglet projects are designed to be legible to AI agents out of the box. The metadata that makes the protocol work for machines also makes it work for your coding assistant. This page covers what your agent gets from the structure — and how to make the most of it.

## CLAUDE.md — Auto-Generated Agent Context

When you run `aglet init`, a `CLAUDE.md` is generated in your project root alongside `domain.yaml` and `intent.md`. You don't write it — it's part of the scaffold.

The file is intentionally lean:

```markdown
# my-project

This is an Aglet project.

Aglet is a protocol for self-describing, agent-native computation...

## Full Specification

https://trevorschoeny.github.io/aglet/

Read the spec before creating, modifying, or scaffolding any Blocks...

## Quick Reference
...

## CLI
...
```

Claude Code reads this automatically at the start of every session. It gives your agent:
1. The identity of the project
2. A pointer to the full specification at the docs site
3. A quick reference for unit types and CLI commands

The full spec lives at [trevorschoeny.github.io/aglet](https://trevorschoeny.github.io/aglet/) — not embedded in the file. This keeps `CLAUDE.md` stable across versions, and keeps the spec in one canonical place.

### Other Agents

For agents that use different context files, the content is the same — copy from your `CLAUDE.md`:

- **Cursor** — `.cursorrules`
- **GitHub Copilot** — `.github/copilot-instructions.md`
- **Any MCP-compatible agent** — can read `CLAUDE.md` directly via file access

## What Your Agent Gets from the Metadata

An Aglet project is self-describing. Your agent doesn't need external documentation or tribal knowledge to understand the codebase. Here's what each file gives it:

### block.yaml — What a Unit Does

```yaml
name: SentimentAnalyzer
description: "Analyzes the sentiment of a text input"
runtime: reasoning
schema:
  in:
    type: object
    properties:
      text: { type: string }
    required: [text]
  out:
    type: object
    properties:
      sentiment: { type: string, enum: [positive, negative, neutral] }
      confidence: { type: number }
      reasoning: { type: string }
    required: [sentiment, confidence, reasoning]
```

Your agent can read this and immediately know: this Block takes text, returns a sentiment classification with confidence and reasoning, and it runs on an LLM. No need to read the implementation to understand the interface.

### intent.md — Why a Unit Exists

```markdown
# SentimentAnalyzer

Classifies user feedback sentiment so the dashboard can surface
trends and flag negative experiences for human review.
```

Intent gives your agent the *context* it needs to make good decisions. When it's modifying a Block, it knows the purpose. When it's generating tests, it knows the edge cases that matter. When it's suggesting architecture changes, it knows which units are load-bearing and which are auxiliary.

### calls Edges — How Units Connect

```yaml
calls:
  - Enricher
  - Validator
```

Your agent can follow `calls` edges to build a mental map of the data flow. It knows which Blocks feed into which, where pipelines start and end, and how changes to one Block's output schema might ripple through the system.

### behavioral_memory — How a Unit Behaves in Practice

The AML (Adaptive Memory Layer) accumulates runtime data into the `behavioral_memory` section of `block.yaml`. Your agent can read this to understand the *operational reality* of each Block — not just what it was designed to do, but what it actually does under load.

```yaml
behavioral_memory:
  total_calls: 847
  avg_runtime_ms: 24.3
  error_rate: 0.0012
  warmth_score: 0.91
  warmth_level: hot
  last_called: "2026-03-17T21:09:05Z"
  token_avg: 1240
  observed_callees:
    ParseDate: 423
    ExtractEntities: 847
  observed_callers:
    TestHarness: 42
```

Key signals for your agent:

- **`warmth_level`** — `hot`, `warm`, or `cold`. Hot Blocks are running in production right now — changes carry more risk. Cold Blocks may be safe to refactor or remove without disruption.
- **`error_rate`** — a high error rate signals something worth investigating before making further changes.
- **`observed_callees`** — which tools this reasoning Block actually uses at runtime. Use this to understand real dependencies, not just declared ones.
- **`observed_callers`** — what depends on this Block. Use this to assess the blast radius of a change.
- **`token_avg`** — for reasoning Blocks, how expensive each call is. Useful for identifying prompt optimization candidates.

See the [Adaptive Memory Layer spec](/spec/aml) for the full picture.

### surface.yaml Contracts — What the Frontend Needs

```yaml
contract:
  dependencies:
    Sentiment:
      block: SentimentAnalyzer
      callers: [FeedbackPanel]
      trigger: user-action
```

When your agent is working on frontend code, the contract tells it exactly which backend capabilities are available, what data shapes to expect, and which Components are responsible for which calls.

## Agent Workflows

### Structural Validation

Your agent can run `aglet validate` to check that the design layer and code layer are in sync. If someone renamed a Block but forgot to update its `calls` reference, or if a Component claims to consume a dependency that doesn't exist in the Surface contract, validate catches it.

This is especially useful after your agent makes changes. A quick `aglet validate` confirms that the structural integrity of the project is intact.

### Deep Review

`aglet validate --deep` generates a judgment-based checklist for your agent to act on. It doesn't call an LLM itself — it produces a structured prompt that tells your agent exactly what to evaluate:

```bash
aglet validate --deep
```

Output is a categorized checklist with specific questions and file references for each Block, Surface, and Component. Categories include intent accuracy, schema correctness, prompt quality (for reasoning Blocks), single responsibility, and contract completeness.

Your agent reads this output and performs the actual review. This keeps the CLI dependency-free while enabling genuine architectural analysis.

```bash
# JSON output for programmatic agent consumption
aglet validate --deep --json

# Filter to a specific unit
aglet validate --deep --unit SentimentAnalyzer
```

### Behavioral Analysis

Before making changes to a high-traffic Block, your agent should inspect its behavioral memory:

```bash
aglet stats SentimentAnalyzer --json
```

This gives a complete picture of the Block's runtime behavior — warmth, error rate, observed dependencies, token usage — that your agent can use to calibrate how careful to be.

## The Philosophy

Aglet doesn't replace your agent. It enables it.

Most codebases are opaque to AI agents. The agent has to read implementation code, infer intent, guess at data flow, and hope that comments are accurate. Every project becomes an archaeology exercise. Aglet inverts this: the codebase speaks for itself. Transparency over abstraction. Emergence over configuration.

Your agent already knows how to read YAML, follow references, and generate code that matches a schema. Aglet just gives it the structured metadata to work with. The result is an agent that understands your project at the architectural level, not just the code level.

This is a deliberate design choice: Aglet will never build its own agent. Every developer already has one they trust. Aglet's role is to make the *project itself* legible — so any agent, present or future, can read the system like a book and operate with full understanding.
