---
title: Runtime Architecture
---

# Runtime Architecture

This page describes how blocks execute. The three-layer architecture — domain listener, block wrapper, block implementation — is specific to blocks. Other unit types participate in the runtime differently:

| Unit | Role in runtime |
|---|---|
| **Domain** | Listener — routes incoming requests to block wrappers |
| **Block** | Wrapper + implementation — executes logic, handles observability and network communication |
| **Surface** | SDK — client-side interaction tracking; receives log entries from block wrappers for contract calls |
| **Component** | Identified by the SDK via headers on contract calls |

Surfaces and components don't have wrappers. They live in the browser. Their observability comes from the client-side SDK (interaction tracking) and from block wrappers (which log surface context when a contract call comes through).

## The Three Layers

### Domain Listener

A domain with `listen: true` in its `domain.yaml` runs a lightweight, always-on HTTP process on a single port. Its job is routing: accept incoming requests and forward them to the correct block wrapper within the domain.

The listener does not execute blocks. It does not write logs. It is a router.

```yaml
# domain.yaml
name: intelligence
listen: true

peers:
  payments: "https://payments.myapp.com"
  auth: "https://auth.internal:8081"
```

When a request arrives for a block that isn't in this domain, the listener looks up the destination domain in `peers` and forwards the request there.

### Block Wrapper

Every block has a wrapper. The wrapper is serverless — it doesn't exist until called. When the domain listener forwards a request, the wrapper spins up and does this:

1. Reads `block.yaml` — `observe`, `calls`, `tools`
2. Pre-warms downstream block wrappers from `calls` (concurrent with step 3)
3. Logs `block.start` to `.aglet/{blockName}/logs.jsonl`
4. Executes the block implementation
5. Logs `block.complete` to `.aglet/{blockName}/logs.jsonl`
6. Forwards output to the relevant downstream block wrappers
7. Returns the result

The wrapper also captures any log lines written by the block implementation itself (stderr output) and appends them to `logs.jsonl`. After execution, it incrementally updates the block's vitals in `.aglet/{blockName}/vitals.json` — call count, running average duration, error rate, warmth.

If the request includes surface context (which surface and component initiated the call), the wrapper writes a `contract.call` entry to the surface's `.aglet/{surfaceName}/logs.jsonl`.

### Block Implementation

The actual code — `main.py`, `main.go`, `prompt.md`, or a `.wasm` module. Reads JSON from stdin, writes JSON to stdout, exits. Knows nothing about the wrapper, the listener, the network, or the Aglet protocol. Pure function.

If the domain declares `stores`, the wrapper injects `AGLET_STORE_{NAME}` environment variables into the process before execution. The implementation reads these to connect to databases using its own preferred library. No Aglet SDK needed — just standard env vars.

## Implementation vs. Wrapper

This is a key distinction.

The **implementation** is self-contained. It's the pure function that transforms input to output. It can be tested in isolation, moved between projects, compiled to WASM. It has no dependencies on Aglet infrastructure.

The **wrapper** is the block's network-facing layer. It reads the block's `block.yaml`, handles observability, communicates with other blocks' wrappers, writes to logs, and participates in pipelines. It interacts with other units — writing to surface logs, pre-warming downstream blocks, forwarding output along declared `calls` edges.

The implementation doesn't know the wrapper exists. The wrapper knows everything the block has declared about itself.

## Routing

### Local Routing

Within a domain, the listener forwards requests directly to block wrappers. No network hop — it's a function call within the same process.

### Cross-Domain Routing

When a block's `calls` or `tools` reference a block in another domain, the wrapper routes through the `peers` table in `domain.yaml`. Each entry maps a domain name to a network address.

```yaml
peers:
  payments: "https://payments.myapp.com"
  auth: "https://auth.internal:8081"
```

In dev, peers might all be `localhost` on different ports. In prod, they're real addresses. The routing logic is identical.

### Pipeline Propagation

When a block finishes executing, its wrapper reads the `calls` field and forwards the output directly to the next block's wrapper — without going back through the domain listener. The signal propagates through the chain, wrapper to wrapper.

```
Request → Listener → BlockA wrapper → BlockB wrapper → BlockC wrapper → result
```

The listener is only involved at the entry point. After that, the wrappers handle the chain.

## Pre-warming

When a wrapper starts executing a block, it concurrently spins up the wrappers for all blocks declared in `calls`. By the time the block finishes, the downstream wrappers are already alive and waiting. Zero cold-start delay on the handoff.

The AML's warmth scores can inform pre-warming strategy:

- **Hot blocks** (warmth ≥ 0.7) — wrappers stay alive between calls rather than spinning down after each invocation
- **Warm blocks** (warmth ≥ 0.3) — wrappers spin up on demand but pre-warm quickly
- **Cold blocks** (warmth < 0.3) — fully serverless, spin up only when called

This means frequently-used pipelines stay warm automatically, while rarely-used blocks consume zero resources.

## Surface Contract Calls

When a surface component calls a block through a contract endpoint, the request flows through the same three layers. The SDK adds headers identifying the surface and component:

```
X-Aglet-Surface: Dashboard
X-Aglet-Caller: FeedbackPanel
```

The domain listener routes to the block wrapper. The wrapper executes the block, writes to the block's `.aglet/` logs as usual, and also writes a `contract.call` entry to the surface's `.aglet/` logs — including which component made the call, duration, and success/error.

The wrapper can do this because the wrapper is the block's network-facing layer. Interacting with other units (including writing to a surface's log) is part of its role.

## Dev/Prod Parity

The architecture is identical in dev and prod:

1. Domain listener runs on a port
2. Routes requests to block wrappers
3. Wrappers execute blocks, log everything
4. Cross-domain calls go through `peers`

The only difference is the addresses in `peers` — `localhost:8081` in dev, `https://payments.myapp.com` in prod. Same binary, same behavior, same logging.

## Observe Contract

Blocks can declare what observability they want in `block.yaml`:

```yaml
observe:
  events: [start, complete, error, tool.call]
```

The wrapper reads this declaration and logs the specified events to `.aglet/{blockName}/logs.jsonl`. The log path is derived from the domain's `.aglet/` directory — not declared in the observe config. Any execution environment that implements the Aglet wrapper protocol reads the same declaration and produces the same logs.
