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

### Stores

```yaml
stores:
  main:
    driver: postgres
    dsn: ${DATABASE_URL}
  cache:
    driver: redis
    dsn: ${REDIS_URL}
```

Declares database and store connections available to Blocks in this domain. The wrapper resolves these at runtime and injects `AGLET_STORE_{NAME}` environment variables into process Blocks before execution. Developers use their own database libraries to connect — Aglet manages the wiring, not the queries.

| Field | Required | Description |
|-------|----------|-------------|
| `driver` | No | Informational label for the store type: `postgres`, `mysql`, `sqlite`, `redis`, `mongo`, `dynamodb`, etc. Helps humans and agents understand what kind of store this is. Not used by the runtime. |
| `dsn` | Yes | Connection string. Use `${ENV_VAR}` references so secrets stay out of YAML. The wrapper resolves these against the process environment at runtime. |

The environment variable convention is `AGLET_STORE_{NAME}` in uppercase. A store named `main` becomes `AGLET_STORE_MAIN`. A store named `cache` becomes `AGLET_STORE_CACHE`. The Block's implementation reads the env var and connects with whatever library it prefers:

```go
dsn := os.Getenv("AGLET_STORE_MAIN")
db, err := pgx.Connect(ctx, dsn)
```

```python
dsn = os.environ["AGLET_STORE_MAIN"]
conn = psycopg.connect(dsn)
```

No Aglet SDK required. No ORM opinions. Just environment variables — the universal interface every database library already accepts.

Stores inherit through the domain chain using the same merge-per-key pattern as runners: nearest domain takes precedence per store name, parent domains fill in any stores not defined locally. A sub-domain can override a parent's `main` store while inheriting its `cache` store.

`aglet validate` warns if a DSN doesn't contain a `${...}` reference (possible hardcoded secret) and if a driver value is unrecognized.

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

### Listener

```yaml
listen: true
```

Opt-in. When set, this domain can be deployed as a domain listener -- a lightweight HTTP server that accepts requests and routes them to block wrappers within the domain. The listener is the domain's membrane: the way signals enter and leave.

In dev and prod, the listener behaves identically. Same binary (`aglet listen`), same routing, same observability. This is what makes dev/prod parity possible.

Domains without `listen: true` can still contain blocks that are executed via `aglet run` or called as tools by other blocks. The `listen` flag only controls whether the domain exposes an HTTP interface.

### Peers

```yaml
peers:
  payments: "https://payments.myapp.com"
  auth: "http://localhost:8081"
```

Cross-domain routing table. Maps domain names to the URLs of their listeners. When a block's `calls` reference a domain-qualified name (e.g., `payments/PaymentAuth`), the wrapper extracts the domain prefix, looks it up in `peers`, and forwards the request to that URL.

Peers are explicit and declared -- no service discovery, no magic. The routing topology is visible in the YAML.

In dev, peers point to localhost ports. In prod, they point to deployed URLs. The block code never changes.

### Aglet Config

```yaml
aglet:
  sink: local
```

Runtime data configuration for the domain. Currently supports `sink` -- where logs and vitals are forwarded.

| Value | Description |
|-------|-------------|
| `local` | Default. Runtime data stays in `.aglet/` only. |
| A URL | After local write, log entries are forwarded to this endpoint in a fire-and-forget goroutine. |

Blocks can override the domain sink with a `sink` field in their own `block.yaml`. Resolution walks the chain: block → nearest domain → parent domain → root domain.

### .aglet/ Directory

Every domain has a `.aglet/` directory that stores runtime data -- logs and vitals -- separate from source code. It has its own git repository for versioned behavioral history.

```
ingest/                          # domain
  .aglet/                        # runtime data (own git repo)
    .git/
    .gitignore                   # ignores **/logs.jsonl
    ParseURL/
      logs.jsonl                 # raw events (not committed)
      vitals.json                # compiled vitals (committed)
    WordCount/
      logs.jsonl
      vitals.json
  ParseURL/                      # block source (main repo)
    block.yaml
    intent.md
    main.py
```

Created automatically by `aglet init` and `aglet new domain`. `aglet snapshot` commits vitals files in all `.aglet/` repos.

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

# stores:
#   main:
#     driver: postgres
#     dsn: ${DATABASE_URL}

listen: true

# peers:
#   payments: "https://payments.myapp.com"
#   auth: "http://localhost:8081"

aglet:
  sink: local
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

## When to Create a Sub-Domain

Create a sub-domain when a group of Blocks has distinct configuration needs (different default model, different error policy), distinct deployment needs (separate listener, separate peers), or distinct organizational identity (different team, different intent).

Don't create sub-domains for purely structural grouping — folders work fine for that. A sub-domain should carry its own `intent.md` with a coherent reason for existing as a separate unit. If you can't write that intent, it's probably just a folder.

## Parent Field and Directory Structure

**The domain hierarchy is declared in YAML, not derived from folder nesting.** The `parent` field in `domain.yaml` is the source of truth. Folders should mirror the declared hierarchy for readability, but if they diverge, the YAML is correct and the folder should be moved to match.

`aglet validate` checks that `parent` references an existing domain and can auto-fix the parent field by inferring it from filesystem nesting when the declared parent is invalid.

## Fractal Composability

Because the root is just a domain, any Aglet project can be absorbed into a larger one. If your application turns out to be a subsystem of something bigger:

1. Give your root `domain.yaml` a `parent` field.
2. It slots into the larger domain hierarchy naturally.

Nothing restructures. The root intent becomes a sub-domain intent. All edges, configs, and inheritance chains continue to work.
