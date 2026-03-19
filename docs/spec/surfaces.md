---
title: Surfaces
---

# Surfaces

A Surface is a stateful, executable application — the boundary where the system meets the outside world. It is an entire deployable frontend: a web app, mobile app, desktop app, or admin dashboard. A Surface is started by its own build/runtime tools (Vite, Tauri, etc.), not by the `aglet` CLI.

A Surface directory is identified by the presence of a `surface.yaml` file. A project may have multiple Surfaces. They may consume the same Block pipelines (via the contract) but are independent executables.

A Surface is not a component, a view, or a widget. It is an entire application. A Surface is not for backend logic — APIs, data pipelines, and webhook handlers are Block graphs.

## surface.yaml Schema

### Identity Fields

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Typed UUID with `s-` prefix. |
| `name` | Yes | PascalCase. Must match the folder name. |
| `description` | Yes | One-line summary. |
| `domain` | Yes | Which domain this Surface belongs to. |
| `version` | Yes | Semver (e.g., `0.1.0`). Surfaces are independently deployable. |

### File Fields

| Field | Required | Description |
|-------|----------|-------------|
| `entry` | Yes | Bootstrap file (e.g., `./main.tsx`). |

### Runtime Fields

| Field | Required | Description |
|-------|----------|-------------|
| `framework` | Yes | Frontend framework (`react`, `vue`, `svelte`, etc.). |
| `bundler` | Yes | Build tool (`vite`, `webpack`, `esbuild`, etc.). |

### Dev Fields

```yaml
dev:
  command: "npm run dev"
  port: 5173
```

### SDK Fields

```yaml
sdk:
  flush_interval: 300    # Event flush interval in seconds (default: 300)
```

| Field | Required | Description |
|-------|----------|-------------|
| `sdk.flush_interval` | No | How often the `@aglet/sdk` flushes client-side events to the domain listener. Default: 300 seconds (5 minutes). |

The domain listener reads this section and injects it into the HTML as `window.__AGLET__` so the SDK auto-configures. See the [Observability](#observability) section.

### Full Example

```yaml
id: s-3a1f8c42-9b7d-4e5a-b2c1-d8f6e4a39012
name: TrevMailClient
description: "Email client frontend"
domain: my-app
version: 0.1.0

entry: ./main.tsx
framework: react
bundler: vite

dev:
  command: "npm run dev"
  port: 5173

sdk:
  flush_interval: 300

contract:
  GetEmailsByCategory:
    block: FetchEmails
    callers: [messages/ConversationList, notifications/NotificationList]
    input:
      type: object
      properties:
        category:
          type: string
          enum: [message, notification, feed, identity]
      required: [category]
    output:
      type: array
      items:
        type: object
        properties:
          nylas: { type: object }
          metadata: { type: object }
        required: [nylas, metadata]

  ArchiveEmail:
    pipeline: ValidateArchive
    callers: [notifications/NotificationCard, messages/ChatView]
    input:
      type: object
      properties:
        email_id: { type: string }
      required: [email_id]
    output:
      type: object
      properties:
        success: { type: boolean }
      required: [success]

  events:
    email.opened:
      emitters: [messages/ChatView, feed/FeedArticle]
      payload:
        type: object
        properties:
          email_id: { type: string }
          timestamp: { type: string, format: date-time }
        required: [email_id, timestamp]
```

## The Contract

The contract is the bridge between the Surface world and the Block world. It lives in `surface.yaml` under the `contract:` key and declares every external data dependency the Surface has.

### Why the Contract Matters

The contract creates full traceability: **Component → contract entry → backend Block(s)**.

- When someone modifies a backend Block, they trace forward through the contract to know which Components are affected.
- When a frontend developer needs server data, they add a contract entry, and the backend team knows exactly what to build.
- By the time the frontend is complete, the contract is a comprehensive specification for the backend.

### Dependency Entry Structure

| Field | Description |
|-------|-------------|
| `block` | Maps to a single Block. Mutually exclusive with `pipeline`. |
| `pipeline` | Maps to a pipeline chain starting at the named Block. Mutually exclusive with `block`. |
| `callers` | Components that invoke this dependency. Format: `domain/ComponentName`. |
| `input` | JSON Schema defining what the Surface sends. |
| `output` | JSON Schema defining what the Surface expects back. |

### Events

Events live under a nested `events:` key. Each event has `emitters` (Components that emit it) and `payload` (JSON Schema for the event data).

### How the Listener Uses the Contract

The domain listener reads the contract and maps each dependency to an HTTP endpoint:

- Each dependency becomes `POST /contract/<DependencyName>`.
- `block: BlockName` executes a single Block.
- `pipeline: StartBlock` follows the `calls` graph through the pipeline chain.
- The Surface makes standard HTTP requests — it never knows whether the dev listener or production infrastructure is answering.

In production, the contract tells whatever infrastructure adapter you use how to map routes to Block executions.

## Observability

Surfaces have their own `logs.jsonl` in `.aglet/{surfaceName}/logs.jsonl`. This file captures two kinds of events:

**Contract call events** — written by block wrappers. When a component calls a block through a contract endpoint, the block wrapper writes a `contract.call` entry to the surface's log with the component name, duration, and success/error. This is automatic — it happens server-side whenever the request includes the right headers.

**Client-side events** — written by the `@aglet/sdk`. Mount/unmount lifecycle and custom tracking events. Buffered in the browser and flushed every 5 minutes + on page unload.

### The SDK

The `@aglet/sdk` package provides per-component instances with three capabilities:

1. **Lifecycle** — `mount()` and `unmount()` log when a component appears and disappears
2. **Contract calls** — `call()` makes a contract request with automatic `X-Aglet-Caller` and `X-Aglet-Surface` headers
3. **Custom tracking** — `track()` logs any component-specific event

```typescript
import { createAglet } from '@aglet/sdk'

const aglet = createAglet('FeedbackPanel')

aglet.mount()
const result = await aglet.call('Sentiment', { text })
aglet.track('analysis_complete', { confidence: 0.95 })
aglet.unmount()
aglet.destroy()
```

In React:

```typescript
function FeedbackPanel() {
  useEffect(() => {
    const aglet = createAglet('FeedbackPanel')
    aglet.mount()
    return () => {
      aglet.unmount()
      aglet.destroy()
    }
  }, [])
}
```

All instances share a single event buffer and flush timer. The SDK has no DOM interaction — mount and unmount are explicit calls. `aglet new component` scaffolds the lifecycle boilerplate automatically.

### How Contract Calls Are Tracked

When a component calls `aglet.call('Sentiment', { text })`, the SDK sends `POST /contract/Sentiment` with two headers:

```
X-Aglet-Caller: FeedbackPanel
X-Aglet-Surface: Dashboard
```

The domain listener routes to the block wrapper. The wrapper executes the block, logs to the block's `.aglet/` logs, and writes a `contract.call` entry to the surface's `.aglet/` logs:

```jsonl
{"event":"contract.call","contract":"Sentiment","block":"SentimentAnalyzer","caller":"FeedbackPanel","surface":"Dashboard","duration_ms":42,"success":true,"ts":"2026-03-17T21:09:00Z"}
```

A component using raw `fetch()` instead of `aglet.call()` still works — the block executes normally — but the surface log won't have the component attribution.

### Client-Side Event Flushing

Events from `mount()`, `unmount()`, and `track()` are buffered and flushed to `POST /_aglet/events` on the domain listener. Flushing happens every `flush_interval` seconds and on page unload via `sendBeacon`. If the endpoint isn't available, the flush silently fails — observability never breaks the app.

## The Logic Division

Within a Surface, there are two kinds of logic. Keeping them separated is essential.

**Orchestration logic → Components.** Deciding *when* to do things. "When the user clicks send, validate the input, call the API, update the list." Inherently stateful.

**Transformation logic → Embedded Blocks.** Computing *what* things are. "Given this raw email body, strip the signature and return clean text." Stateless. Pure function: data in, data out.

The boundary is the function call. A Component handles orchestration. An embedded Block handles computation. The Component calls the Block and gets a result. The Block didn't touch state. The Component didn't compute the transformation.

For details on Components, their YAML schema, and the consumes field, see [Components](/spec/components).
