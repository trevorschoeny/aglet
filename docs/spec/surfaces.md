---
title: Surfaces
---

# Surfaces

A Surface is a stateful, executable application -- the boundary where the system meets the outside world. It is an entire deployable frontend: a web app, mobile app, desktop app, or admin dashboard. A Surface is started by its own build/runtime tools (Vite, Tauri, etc.), not by the `aglet` CLI or any Block runner.

A Surface directory is identified by the presence of a `surface.yaml` file.

A project may have multiple Surfaces. They may consume the same Block pipelines (via the contract) but are independent executables.

## What a Surface Is Not

A Surface is **not** a component, a view, or a widget. It is an entire application. If one piece of the UI gets complex, you refactor Components -- you don't create a new Surface.

A Surface is **not** for backend logic. APIs, data pipelines, webhook handlers, and cron jobs are Block graphs. Surfaces exist specifically for stateful, interactive interfaces that face users through a screen. Putting backend logic in a Surface hides functional logic inside a sealed executable where nothing else can call it.

## surface.yaml Schema

`surface.yaml` carries the Surface's identity, runtime configuration, dev settings, and the contract.

### Identity Fields

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Typed UUID with `s-` prefix. Format: `s-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`. |
| `name` | Yes | PascalCase. Must match the folder name. |
| `description` | Yes | One-line summary for CLI output and quick scanning. |
| `domain` | Yes | Which domain this Surface belongs to. |
| `version` | Yes | Semver (e.g., `0.1.0`). Surfaces are independently deployable, so they need version tracking. |

### File Fields

| Field | Required | Description |
|-------|----------|-------------|
| `entry` | Yes | The bootstrap file that starts the application (e.g., `./main.tsx`). |

### Runtime Fields

| Field | Required | Description |
|-------|----------|-------------|
| `framework` | Yes | The frontend framework (`react`, `vue`, `svelte`, etc.). Informational -- Aglet is framework-agnostic, but the entire Surface must use one framework. |
| `bundler` | Yes | The build tool (`vite`, `webpack`, `esbuild`, etc.). |

### Dev Fields

```yaml
dev:
  command: "npm run dev"     # How to start this Surface locally.
  port: 5173                 # Default dev server port.
```

If a project has multiple Surfaces, each declares its own port to avoid conflicts.

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

The contract is the bridge between the Surface world and the Block world. It lives inside `surface.yaml` under the `contract:` key. It declares every external data dependency the Surface has and every outbound event it emits.

### Why the Contract Matters

The contract creates full traceability: **Component -> contract entry -> backend Block(s)**.

- When someone modifies a backend Block, they trace forward through the contract to know which Components are affected.
- When a frontend developer adds a Component that needs server data, they add a contract entry, and the backend team knows exactly what to build.
- By the time the frontend is complete, the contract is a comprehensive specification for the backend.

### Dependency Entry Structure

Each entry under `contract:` has the following fields:

| Field | Description |
|-------|-------------|
| `block` | Maps the dependency to a single Block. Mutually exclusive with `pipeline`. |
| `pipeline` | Maps the dependency to a pipeline chain starting at the named Block. Mutually exclusive with `block`. |
| `callers` | List of Components that invoke this dependency. Format: `domain/ComponentName`. |
| `input` | JSON Schema (draft-07) in YAML syntax defining what the Surface sends. |
| `output` | JSON Schema (draft-07) in YAML syntax defining what the Surface expects back. |

If neither `block` nor `pipeline` is present, `aglet serve` falls back to looking for a Block whose name matches the dependency name. Prefer explicit mappings for clarity.

### Events

Events live under a nested `events:` key within the contract.

| Field | Description |
|-------|-------------|
| `emitters` | List of Components that emit this event. |
| `payload` | JSON Schema (draft-07) in YAML syntax defining the event payload. |

### How `aglet serve` Uses the Contract

During local development, `aglet serve` reads the contract and maps each dependency to an HTTP endpoint:

- Each dependency becomes `POST /contract/<DependencyName>`.
- `block: BlockName` executes a single Block.
- `pipeline: StartBlock` follows the `calls` graph from the start Block through the pipeline chain.
- The Surface makes standard HTTP requests -- it never knows whether `aglet serve` or production infrastructure is answering.
- CORS headers are included automatically.

In production, the `block` and `pipeline` fields tell whatever infrastructure adapter you use (API Gateway, serverless, etc.) how to map routes to Block executions.

### Keeping the Contract Current

Update the contract in real time as the Surface evolves. When a Component starts needing new data, add a contract entry immediately -- not later. When a dependency changes shape, update the contract in the same pass. The contract drifting from reality is as dangerous as an `intent.md` drifting from its Block's code.

## Components

A Component is a stateful unit within a Surface. Components handle orchestration logic: responding to user events, managing state transitions, coordinating effects, deciding *when* things happen. Components are the building material of Surfaces.

A Component directory is identified by the presence of a `component.yaml` file.

### component.yaml Schema

```yaml
# === Identity ===
id: c-e2d4b6a8-1c3f-5d7e-9a0b-2e4f6d8c0a1b
name: ConversationList
description: "Displays conversations grouped by category"
domain: messages
role: list

# === Contract ===
consumes:
  - GetEmailsByCategory
  - MarkEmailRead

# === Permissions ===
permissions:
  user: []
  developer: []

# === Analytics ===
analytics:
  track_render: false
  track_interaction: false
```

### Identity Fields

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Typed UUID with `c-` prefix. |
| `name` | Yes | PascalCase. Must match the folder name. |
| `description` | Yes | One-line summary. |
| `domain` | Yes | Which domain within the Surface this Component belongs to. |
| `role` | Yes | Shape of this Component: `page`, `layout`, `widget`, `form`, `list`, `card`, `modal`, `nav`, `input`, etc. Not a closed set. |

### Contract Fields

| Field | Description |
|-------|-------------|
| `consumes` | List of contract dependency names from the parent Surface's `surface.yaml`. |

The `consumes` field creates bidirectional traceability with the Surface's contract. The contract lists which Components call each dependency (via `callers`), and the Component lists which contract entries it uses (via `consumes`). If they disagree, that's a sync check failure. `aglet validate` auto-fixes this drift.

### Permissions and Analytics

| Field | Description |
|-------|-------------|
| `permissions.user` | End-user visibility/access permissions (e.g., `["premium", "admin"]`). |
| `permissions.developer` | Developer editing permissions (e.g., `["senior", "frontend-lead"]`). |
| `analytics.track_render` | Whether to log render frequency. |
| `analytics.track_interaction` | Whether to log user interactions. |

### What Components Do

**Components handle orchestration logic.** They decide *when* things happen: when to fetch data, when to update state, when to trigger navigation, when to call an embedded Block.

**Components do not handle transformation logic.** When a Component needs data transformed -- parsed, formatted, validated, filtered, sorted -- it calls an embedded Block. The Component passes data in and receives transformed data back. The Block did the computation; the Component orchestrated the flow.

### The Extraction Litmus Test

"Would this logic be useful outside this Component?" If yes, extract it into an embedded Block with typed schemas and an intent doc. If no -- if it's genuinely about managing this specific piece of UI state -- it's Component logic.

Trivial one-liner derivations (like computing a count from a filtered list) can stay in Components. The threshold for extraction is whether the logic has enough complexity or reusability to warrant an intent doc.

## The Logic Division

Within a Surface, there are two fundamentally different kinds of logic. Keeping them separated is essential.

**Orchestration logic -> Components.** Deciding *when* to do things. "When the user clicks send, validate the input, call the API, and update the conversation list." This is inherently stateful -- it responds to events, reads current state, and triggers transitions.

**Transformation logic -> Embedded Blocks.** Computing *what* things are. "Given this raw email body, strip the signature and return clean text." This is stateless. It doesn't care about events or timing. Pure function: data in, data out.

The boundary between them is the function call. A Component handles the orchestration (the user typed a message, read the current conversation from state, need to format the new message). The formatting itself is an embedded Block call. The Component calls `formatMessage(rawInput)` and gets back a display-ready object. The Block didn't touch state. The Component didn't compute the transformation.
