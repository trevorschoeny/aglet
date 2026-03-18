---
title: Components
---

# Components

A Component is a stateful unit within a Surface. Components handle orchestration logic: responding to user events, managing state transitions, coordinating effects, deciding *when* things happen. Components are the building material of Surfaces.

A Component directory is identified by the presence of a `component.yaml` file.

## component.yaml Schema

```yaml
id: c-e2d4b6a8-1c3f-5d7e-9a0b-2e4f6d8c0a1b
name: ConversationList
description: "Displays conversations grouped by category"
domain: messages
role: list

consumes:
  - GetEmailsByCategory
  - MarkEmailRead
```

### Identity Fields

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Typed UUID with `c-` prefix. |
| `name` | Yes | PascalCase. Must match the folder name. |
| `description` | Yes | One-line summary. |
| `domain` | Yes | Which domain within the Surface this Component belongs to. |
| `role` | Yes | Shape of this Component: `page`, `layout`, `widget`, `form`, `list`, `card`, `modal`, `nav`, `input`, etc. Not a closed set. |

### The consumes Field

```yaml
consumes:
  - GetEmailsByCategory
  - MarkEmailRead
```

`consumes` lists which contract dependencies (from the parent Surface's `surface.yaml`) this Component uses. This creates bidirectional traceability:

- The Surface contract lists which Components call each dependency (via `callers`)
- The Component lists which contract entries it uses (via `consumes`)

If they disagree, `aglet validate` catches it and auto-fixes the drift.

## SDK Wiring

When scaffolded with `aglet new component`, the generated `.tsx` file includes the SDK lifecycle:

```typescript
import { useEffect } from "react";
import { createAglet } from "@aglet/sdk";

interface ConversationListProps {}

export function ConversationList({}: ConversationListProps) {
  useEffect(() => {
    const aglet = createAglet("ConversationList");
    aglet.mount();
    return () => {
      aglet.unmount();
      aglet.destroy();
    };
  }, []);

  return <div>ConversationList</div>;
}
```

The `aglet` instance provides:
- `mount()` / `unmount()` — lifecycle tracking
- `call(contract, input)` — contract calls with automatic `X-Aglet-Caller` header
- `track(action, detail)` — custom event tracking
- `flush()` / `destroy()` — buffer management

See the [Surface Observability](/spec/surfaces#observability) section for how these events are logged.

## What Components Do

**Components handle orchestration logic.** They decide *when* things happen: when to fetch data, when to update state, when to trigger navigation, when to call an embedded Block.

**Components do not handle transformation logic.** When a Component needs data transformed — parsed, formatted, validated, filtered, sorted — it calls an embedded Block. The Component passes data in and receives transformed data back. The Block did the computation; the Component orchestrated the flow.

## The Extraction Litmus Test

"Would this logic be useful outside this Component?" If yes, extract it into an embedded Block with typed schemas and an intent doc. If no — if it's genuinely about managing this specific piece of UI state — it's Component logic.

Trivial one-liner derivations (computing a count from a filtered list) can stay in Components. The threshold: does the logic have enough complexity or reusability to warrant an intent doc?
