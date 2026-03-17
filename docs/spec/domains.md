---
title: Domains
---

# Domains

A domain is a directory containing a `domain.yaml` file. Domains are organizational groupings -- they contain Blocks, Surfaces, Components, and sub-domains. They carry configuration defaults and define scope boundaries. Domains have no execution semantics. They are pure structure.

Everything in Aglet exists within a domain. The root directory of the project is itself a domain. There is no separate "project" concept. It's domains all the way down.

Each domain contains:

- `domain.yaml` -- Required. Declares the domain's name, parent, and default configs.
- `intent.md` -- Required. The domain's founding intent document. See the [Intent](/spec/intent) specification.

## domain.yaml Schema

### Identity Fields

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Typed UUID with `d-` prefix. Format: `d-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`. |
| `name` | Yes | Must match the folder name. |
| `parent` | No | Name of the parent domain. Omit for the root domain. |

### Entrypoints

```yaml
entrypoints:
  - TrevMailClient        # A Surface (frontend entry)
  - EmailClassifier       # A Block pipeline (backend entry)
```

Optional, root domain only. Semantic -- tells humans and tooling where the designed-for front doors of the system are. Both Surfaces and Blocks can be entrypoints. The CLI can still run any Block directly regardless of whether it's listed.

### Runners

```yaml
runners:
  .go: "go run"
  .ts: "npx tsx"
  .py: "python3"
```

Maps file extensions to the command that executes them. Adding a new language to the project means adding one line here.

The current CLI reads runners only from the root `domain.yaml`. Sub-domain runner overrides are planned for a future version.

### Providers

```yaml
providers:
  anthropic:
    env: ANTHROPIC_API_KEY

  openai:
    env: OPENAI_API_KEY

  groq:
    env: GROQ_API_KEY
    url: https://api.groq.com/openai/v1
    format: openai

  local:
    url: http://localhost:11434/v1
    format: openai
```

Tells the `aglet-reason` runner how to authenticate with and call LLM providers for reasoning Blocks.

| Field | Required | Description |
|-------|----------|-------------|
| `env` | No | Environment variable holding the API key. Omit for local providers that don't require auth. |
| `url` | No | Custom API endpoint. Omit for well-known providers (`anthropic`, `openai`) -- the runner has built-in defaults. |
| `format` | No | `"anthropic"` or `"openai"`. Most LLM providers speak one of these two protocols. Omit for well-known providers. |

The runner resolves which provider to use either implicitly from the model name (e.g., `claude-*` -> `anthropic`, `gpt-*` -> `openai`) or explicitly via a `provider` field in a reasoning Block's `block.yaml`.

### Defaults

```yaml
defaults:
  execution: sync          # sync | async
  error: propagate         # propagate | absorb | halt
  model: claude-sonnet-4-20250514
```

| Field | Description |
|-------|-------------|
| `execution` | Default execution mode for process Blocks in this domain. |
| `error` | Default error handling policy for process Blocks in this domain. |
| `model` | Default LLM model for reasoning Blocks in this domain. |

All three cascade through the domain hierarchy and can be overridden at any level.

## The Root Domain

The root domain is the project itself. Its `domain.yaml` defines project-wide settings, and its `intent.md` is the founding document that every Block, Surface, and domain exists in service of.

### Full Root domain.yaml Example

```yaml
id: d-a1b2c3d4-e5f6-7890-abcd-ef1234567890
name: my-app

entrypoints:
  - TrevMailClient
  - EmailClassifier

runners:
  .go: "go run"
  .ts: "npx tsx"
  .py: "python3"

providers:
  anthropic:
    env: ANTHROPIC_API_KEY
  openai:
    env: OPENAI_API_KEY
  # groq:
  #   env: GROQ_API_KEY
  #   url: https://api.groq.com/openai/v1
  #   format: openai
  # local:
  #   url: http://localhost:11434/v1
  #   format: openai

defaults:
  execution: sync
  error: propagate
  model: claude-sonnet-4-20250514
```

### Sub-domain Example

```yaml
id: d-550e8400-e29b-41d4-a716-446655440000
name: international-payments
parent: payments

defaults:
  execution: async
  error: propagate
```

## Config Inheritance Chain

Configuration cascades from root to leaf, with each level able to override:

1. **Root `domain.yaml` defaults** -- project-wide baseline.
2. **Sub-domain `domain.yaml` defaults** -- recursive, as deep as the hierarchy goes.
3. **Block's `block.yaml` or Surface's `surface.yaml`** -- final override.

A Block's `domain` field in its `block.yaml` declares which domain it belongs to. It inherits the full chain from that domain. Any value set explicitly in the Block's own file wins.

This applies to `execution`, `error`, and `model` -- all three cascade and can be overridden at any level.

## Domain Intents

Every domain has an `intent.md`. For domains, the intent is the founding document: it defines what the domain is, who it serves, and what constraints are sacred. Domain intents tend to be broader and more visionary than Block or Component intents.

**Root domain intent example:**

```markdown
# My App

A payment processing system that prioritizes transaction safety
over speed, designed for small merchants who need simple,
auditable checkout flows.

## Sacred Constraints

- Every transaction must be independently verifiable
- No silent failures -- if something breaks, the merchant knows
- Sub-200ms total pipeline latency for the happy path

## Who This Serves

Small business owners who don't have a dedicated payments team
and need to trust that their checkout just works.
```

**Sub-domain intent example:**

```markdown
# Auth

Handles all identity verification and session management. Every
request that touches user data or financial operations passes
through this domain first. Auth is the trust boundary.
```

## Parent Field and Directory Structure

**The domain hierarchy is declared in YAML, not derived from folder nesting.** The `parent` field in `domain.yaml` is the source of truth. Folders should mirror the declared hierarchy for readability, but if they diverge, the YAML is correct and the folder should be moved to match.

`aglet validate` checks that `parent` references an existing domain and can auto-fix the parent field by inferring it from filesystem nesting when the declared parent is invalid.

## Fractal Composability

Because the root is just a domain, any Aglet project can be absorbed into a larger one. If your application turns out to be a subsystem of something bigger:

1. Give your root `domain.yaml` a `parent` field.
2. It slots into the larger domain hierarchy naturally.

Nothing restructures. The root intent becomes a sub-domain intent. All edges, configs, and inheritance chains continue to work.
