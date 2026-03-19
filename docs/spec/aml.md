---
title: Adaptive Memory Layer
---

# Adaptive Memory Layer

The Adaptive Memory Layer (AML) is the behavioral intelligence built into every Block. While the declared layer of a Block -- its `block.yaml`, schemas, and edges -- describes what a Block *is*, the AML describes what a Block *does* in practice. Together they form the Semantic Overlay: a complete, self-describing picture of the unit.

## The Semantic Overlay

Every Block in Aglet carries two kinds of knowledge:

**Declared layer** -- authored by you. Identity, schemas, edges, runtime, intent. This is the design: what the Block promises to do, what it accepts, what it emits, what it connects to.

**Behavioral layer** -- written by the runtime. Call counts, latency distributions, error rates, warmth, observed dependencies. This is the reality: what the Block actually does under load, how often it runs, what tools it actually reaches for.

Together, these form the Semantic Overlay. A Block is no longer just a description of intent -- it's a living artifact that accumulates knowledge about its own behavior and exposes that knowledge to any agent or tool that reads it.

This is the core insight of the AML: **the design layer and the behavioral layer should live in the same file, and they should stay in sync automatically**.

## Behavioral Memory

The AML writes behavioral data into the `behavioral_memory` section of `block.yaml`. It's written by `aglet stats --write`, updated silently after each successful `aglet run`, and readable by any agent or tool that inspects the Block.

```yaml
# AML — written by `aglet stats --write`, do not edit manually
behavioral_memory:
  total_calls: 847
  avg_runtime_ms: 24.3
  error_rate: 0.0012
  warmth_score: 0.91
  warmth_level: hot
  last_called: "2026-03-17T21:09:05Z"
  version_since: "2026-03-10T14:22:00Z"
  token_avg: 1240
  observed_callees:
    ParseDate: 423
    ExtractEntities: 847
  observed_callers:
    TestHarness: 42
  last_updated: "2026-03-17T21:09:10Z"
```

### Fields

| Field | Description |
|-------|-------------|
| `total_calls` | Total number of successful + failed invocations since `version_since`. |
| `avg_runtime_ms` | Mean execution time in milliseconds across all completed calls. |
| `error_rate` | Fraction of calls that ended in error. `0.0` is perfect; `1.0` is total failure. |
| `warmth_score` | A 0.0–1.0 score combining recency and frequency. Used to drive `warmth_level`. |
| `warmth_level` | `hot`, `warm`, or `cold`. Derived from `warmth_score`. |
| `last_called` | ISO 8601 timestamp of the most recent invocation. |
| `version_since` | Timestamp of the most recent `block.updated` event -- when the current measurement window started. Resets on code change. |
| `token_avg` | Average LLM tokens used per call. Reasoning Blocks only; 0 for others. |
| `observed_callees` | Map of tool Block names → call counts. Mining from `tool.call` log events. Reasoning Blocks only. |
| `observed_callers` | Map of Block names → times they called this Block as a tool. Cross-block scan of `logs.jsonl`. |
| `last_updated` | When the `behavioral_memory` section was last rewritten. |

## Warmth

Warmth is a measure of a Block's current operational relevance -- not just how often it runs, but whether it's still actively being used. A Block that ran a million times two years ago and hasn't run since is cold. A Block that ran once yesterday is warm.

The score is computed as:

```
warmth_score = (recency × 0.7) + (frequency × 0.3)
```

**Recency** decays exponentially from the last call:
- Called within the last hour → 1.0
- Called within the last day → ~0.9
- Called within the last week → ~0.7
- Called within the last month → ~0.4
- Called within the last year → ~0.1
- Never called → 0.0

**Frequency** is normalized against a "very active" baseline of 1000 calls, capped at 1.0.

**Warmth levels:**
- `hot` — score ≥ 0.7. In active use.
- `warm` — score ≥ 0.3. Regular but not frequent.
- `cold` — score < 0.3. Infrequently used or dormant.

Warmth is recalculated on every stats write. It's a snapshot of the Block's health at a point in time, not a historical average.

## What logs.jsonl Looks Like

Every Block accumulates log entries in `logs.jsonl`. Each line is one JSON event:

```jsonl
{"event":"block.start","ts":"2026-03-17T21:09:00Z","status":"info","source":"aglet","block":"EmailClassifier","block_id":"b-9f8e7d6c...","runtime":"reasoning","model":"claude-sonnet-4-20250514"}
{"event":"tool.call","ts":"2026-03-17T21:09:01Z","status":"info","source":"aglet","block":"EmailClassifier","tool":"ParseDate","iteration":1}
{"event":"tool.result","ts":"2026-03-17T21:09:01Z","status":"info","source":"aglet","block":"EmailClassifier","tool":"ParseDate","duration_ms":45,"success":true}
{"event":"block.complete","ts":"2026-03-17T21:09:02Z","status":"success","source":"aglet","block":"EmailClassifier","block_id":"b-9f8e7d6c...","duration_ms":2100,"output_bytes":156,"input_tokens":340,"output_tokens":89}
```

The AML reads these events to compute behavioral memory. The wrapper writes them; the AML consumes them.

## Incremental Accumulation

The AML doesn't recount logs from scratch on every run. It uses a checkpoint-and-delta model.

### How it works

1. **Read** the existing `behavioral_memory` from `block.yaml` — the last checkpoint.
2. **Scan** `logs.jsonl` for the most recent `block.updated` event (a code change).
3. **Determine the window:**
   - If no existing memory, or if `block.updated` is newer than `last_updated` → **reset** (start fresh from the code change event).
   - Otherwise → **increment** (process only entries newer than `last_updated`).
4. **Compute** new counts from the delta entries and add them to the checkpoint.
5. **Write** the new snapshot back to `block.yaml`.

### Worked example

A Block has `behavioral_memory` with `total_calls: 100`, `avg_runtime_ms: 25.0`, `last_updated: "2026-03-17T20:00:00Z"`. Since then, three new entries appeared in `logs.jsonl`:

- `block.complete` at 20:30 — duration 30ms
- `block.complete` at 21:00 — duration 20ms
- `block.error` at 21:05

The AML processes only these three entries. New total_calls: 103. New avg_runtime_ms: recomputed from 100 previous calls averaging 25ms plus 2 new completed calls (30ms, 20ms). New error_rate: 1/103. The other 97 entries in the log are untouched.

If the developer then modifies the Block's code and runs it again, the wrapper detects the file hash change, logs `block.updated`, and the next stats computation resets — starting fresh from that point.

This means the AML processes only new log entries on each call. A Block with millions of historical entries and ten new runs since the last stats write processes ten entries, not millions.

### Reset on Code Change

When a Block's implementation changes, the AML detects the change via the `block.updated` event emitted by the runner (which compares file hashes). The measurement window resets to the point of the change, and `version_since` is updated to that timestamp.

This is intentional: behavioral data from the previous version is no longer meaningful for the current version. The Block has changed. Start fresh.

## Observed Edges

The behavioral layer surfaces the Block's *actual* runtime dependency graph, independent of what was declared in `tools` or `calls`.

**Observed callees** (`observed_callees`) are mined from `tool.call` log events in the Block's own `logs.jsonl`. Each entry records which tool Block was called and when. The AML counts these per-window and stores `BlockName → count`. Only reasoning Blocks produce these events.

**Observed callers** (`observed_callers`) require a cross-block scan. To compute them, the AML reads every other Block's `logs.jsonl` and finds `tool.call` events where the `tool` field matches this Block's name. The counts tell you which Blocks are actually depending on this one at runtime.

Cross-block caller scanning only happens during explicit `aglet stats` calls. It's skipped during the silent auto-update after `aglet run` to avoid the O(n) scan cost on every invocation.

### Divergence Detection

`aglet validate` compares observed callees against the declared `tools` field:

- **Undeclared runtime dependency** — a Block appears in `observed_callees` but not in `tools`. The Block is using a tool it never declared. This is a real dependency that should be visible in the design layer.
- **Dead declared tool** — a Block is in `tools` but never appears in `observed_callees` after more than 20 total calls. The Block was declared as a tool but the reasoning never actually uses it. Either the prompt doesn't reach for it, or it's an untested code path.

These checks help surface drift between design intent and runtime reality. Aglet's position is that the declared and behavioral layers should agree. When they don't, that's a signal worth acting on.

## How Agents Use Behavioral Memory

The behavioral layer exists specifically to be read by agents. An AI agent working on an Aglet project can inspect `behavioral_memory` to:

- **Prioritize review effort.** Hot Blocks are running in production right now -- changes to them carry more risk. Cold Blocks may be safe to modify or remove without disruption.
- **Understand actual dependencies.** `observed_callees` shows which tools the reasoning actually uses. `observed_callers` shows what depends on this Block. An agent can use this to assess the blast radius of any change.
- **Detect drift.** If `observed_callees` diverges from `tools`, or if `avg_runtime_ms` is wildly different from what `schema.out` implies, something has changed. The agent can flag it or investigate.
- **Guide optimization.** A Block with high `token_avg` and low confidence in outputs may be a candidate for prompt refinement. A Block with high error rate and low warmth may simply be untested.
- **Calibrate `aglet validate --deep`.** The deep validation checklist is generated with warmth context. A hot Block with no observed callees in a reasoning context gets different checks than a cold utility Block that's never been run.

The behavioral layer makes the invisible visible. Every time a Block runs, the system learns something. The AML ensures that knowledge doesn't evaporate -- it accumulates in the file, attached to the unit it describes, readable by any agent that comes after.

## aglet stats

The `aglet stats` command reads and optionally writes behavioral memory.

```bash
# Stats for a single Block
aglet stats EmailClassifier

# Stats for all Blocks in a domain
aglet stats --domain intelligence

# Project-wide stats
aglet stats --project

# Write results back to block.yaml files
aglet stats EmailClassifier --write

# JSON output (for agent consumption)
aglet stats EmailClassifier --json
```

See the [CLI Reference](/cli/) for full flag documentation.

## Future Features

Features that are designed but not yet implemented. These represent the direction the AML is heading.

- **Custom vitals metrics.** Developers will be able to declare custom metrics in block.yaml or domain.yaml that the wrapper tracks alongside the built-in ones. For example, a reasoning Block could track `avg_confidence` from its output schema, or a process Block could track `avg_output_size_kb`. The wrapper would read the metric definitions and compute them incrementally, same as the built-in vitals.

- **YAML-driven SDK observability.** Surface and component observability config (what to track, flush intervals, interaction types) declared in surface.yaml and component.yaml, automatically injected into the client SDK via the domain listener. No code changes needed to adjust tracking behavior.

- **Warmth-based wrapper cooldown.** Hot blocks (high warmth score) keep their wrappers alive between calls for zero cold-start latency. Cold blocks are fully serverless. The cooldown period would be configurable in the observe contract.

- **WASM compilation.** `aglet build` compiles process blocks to WebAssembly modules for portable, near-instant execution. The wrapper becomes a WASM host instead of a subprocess spawner.

- **Aglet Management System (AMS).** A hosted dashboard that aggregates vitals, logs, and behavioral data across domains, environments, and teams. The `sink` config in domain.yaml would point to the AMS endpoint. Think of it as what GitHub is to git — the collaboration and visibility layer on top of the protocol.
