---
title: Agent Setup
---

# Agent Setup

Aglet projects are designed to be legible to AI agents out of the box. The metadata that makes the protocol work for machines also makes it work for your coding assistant. This page covers how to set up your agent and what it gets from the structure.

## Point Your Agent at the Spec

Add a `CLAUDE.md` (or your agent's equivalent config file) to your project root:

```markdown
This is an Aglet project -- load the Aglet specification from
https://github.com/trevorschoeny/aglet
```

For Claude Code, this file is read automatically at the start of every session. For Cursor, use `.cursorrules`. For Copilot, use `.github/copilot-instructions.md`. The content is the same -- point the agent to the Aglet repo so it can load the full specification.

That's all the setup you need. The rest is already in your project files.

## What Your Agent Gets from the Metadata

An Aglet project is self-describing. Your agent doesn't need external documentation or tribal knowledge to understand the codebase. Here's what each file gives it:

### block.yaml -- What a Unit Does

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

### intent.md -- Why a Unit Exists

```markdown
# SentimentAnalyzer

Classifies user feedback sentiment so the dashboard can surface
trends and flag negative experiences for human review.
```

Intent gives your agent the *context* it needs to make good decisions. When it's modifying a Block, it knows the purpose. When it's generating tests, it knows the edge cases that matter. When it's suggesting architecture changes, it knows which units are load-bearing and which are auxiliary.

### calls Edges -- How Units Connect

```yaml
calls:
  - Enricher
  - Validator
```

Your agent can follow `calls` edges to build a mental map of the data flow. It knows which Blocks feed into which, where pipelines start and end, and how changes to one Block's output schema might ripple through the system.

### surface.yaml Contracts -- What the Frontend Needs

```yaml
contract:
  dependencies:
    Sentiment:
      block: SentimentAnalyzer
      callers: [FeedbackPanel]
      trigger: user-action
```

When your agent is working on frontend code, the contract tells it exactly which backend capabilities are available, what data shapes to expect, and which Components are responsible for which calls.

### aglet validate -- Catching Structural Drift

Your agent can run `aglet validate` to check that the design layer and code layer are in sync. If someone renamed a Block but forgot to update its `calls` reference, or if a Component claims to consume a dependency that doesn't exist in the Surface contract, validate catches it.

This is especially useful after your agent makes changes. A quick `aglet validate` confirms that the structural integrity of the project is intact.

## The Philosophy

Aglet doesn't replace your agent. It enables it.

Most codebases are opaque to AI agents. The agent has to read implementation code, infer intent, guess at data flow, and hope that comments are accurate. Every project becomes an archaeology exercise. Aglet inverts this: the codebase speaks for itself. Transparency over abstraction. Emergence over configuration.

Your agent already knows how to read YAML, follow references, and generate code that matches a schema. Aglet just gives it the structured metadata to work with. The result is an agent that understands your project at the architectural level, not just the code level.

This is a deliberate design choice: Aglet will never build its own agent. Every developer already has one they trust. Aglet's role is to make the *project itself* legible — so any agent, present or future, can read the system like a book and operate with full understanding.

## Works with Any Agent

Aglet's metadata is plain YAML and Markdown. There's nothing Claude-specific or vendor-locked about it. Any agent that can read files can read an Aglet project:

- **Claude Code** -- reads `CLAUDE.md` for project context
- **Cursor** -- reads `.cursorrules` for project context
- **GitHub Copilot** -- reads `.github/copilot-instructions.md` for project context
- **Any MCP-compatible agent** -- can read files directly
- **Custom tooling** -- parse `block.yaml` with any YAML library in any language

The protocol is the same regardless of which agent you use. The YAML describes the system. The agent reads it. That's the whole integration story.
