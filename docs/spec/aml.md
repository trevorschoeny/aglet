---
title: Adaptive Memory Layer
---

# Adaptive Memory Layer

The Adaptive Memory Layer (AML) is the observability intelligence built into every Block. While the declared layer -- `block.yaml`, schemas, edges -- describes what a Block *is*, the AML describes what a Block *does* in practice. Together they form the Semantic Overlay.

## The Semantic Overlay

Every Block carries two kinds of knowledge:

**Declared layer** -- authored by you. Identity, schemas, edges, runtime, intent. This is the design.

**Behavioral layer** -- written by the runtime. Call counts, latency, error rates, warmth, observed dependencies. This is the reality.

Together, these form the Semantic Overlay. The declared layer lives in the Block's source directory (`block.yaml`, `intent.md`). The behavioral layer lives in `.aglet/` -- a separate runtime data directory, one per domain, with its own git history.

## Vitals

The AML computes **vitals** -- a compiled behavioral profile for each Block. Vitals live in `.aglet/{blockName}/vitals.json`, separate from source code. They're updated incrementally after every execution by the block wrapper.

```json
{
  "total_calls": 847,
  "avg_runtime_ms": 24.3,
  "error_rate": 0.0012,
  "warmth_score": 0.91,
  "warmth_level": "hot",
  "last_called": "2026-03-19T08:12:47Z",
  "version_since": "2026-03-10T14:22:00Z",
  "token_avg": 1240,
  "observed_callees": {
    "ParseDate": 423,
    "ExtractEntities": 847
  },
  "observed_callers": {
    "TestHarness": 42
  },
  "last_updated": "2026-03-19T08:12:49Z"
}
```

### Fields

| Field | Description |
|-------|-------------|
| `total_calls` | Total invocations since `version_since`. |
| `avg_runtime_ms` | Mean execution time in milliseconds. |
| `error_rate` | Fraction of calls that errored. 0.0 is perfect. |
| `warmth_score` | 0.0–1.0 combining recency and frequency. |
| `warmth_level` | `hot`, `warm`, or `cold`. Derived from `warmth_score`. |
| `last_called` | ISO 8601 timestamp of the most recent invocation. |
| `version_since` | When the current measurement window started. Resets on code change. |
| `token_avg` | Average LLM tokens per call. Reasoning Blocks only. |
| `observed_callees` | Tool Blocks this Block actually called at runtime. |
| `observed_callers` | Blocks that called this Block as a tool. |
| `last_updated` | When vitals were last written. |

## Where Vitals Live

Vitals and logs live in `.aglet/` at each domain level, **not** in the Block's source directory. Source files stay clean.

```
ingest/                          # domain source
  ParseURL/
    block.yaml                   # declared layer (source, version-controlled)
    intent.md
    main.py
  .aglet/                        # behavioral layer (runtime data, own git repo)
    ParseURL/
      logs.jsonl                 # raw events (gitignored within .aglet/)
      vitals.json                # compiled vitals (committed in .aglet/)
```

The `.aglet/` directory has its own git repository. `aglet snapshot` commits vitals files with a reference to the main repo's HEAD, creating a correlated behavioral history.

## Incremental Updates

Vitals update after every execution with O(1) cost. The wrapper knows what just happened -- duration, success/error, tokens -- and increments the counters directly. No log scanning required for per-run updates.

```
Block executes → wrapper records duration → increments vitals counters → writes vitals.json
```

The full log-scanning path (`aglet stats`) still exists for from-scratch recalculation and cross-block caller scans.

## Warmth

Warmth measures operational relevance -- not just frequency, but recency.

```
warmth_score = (recency × 0.7) + (frequency × 0.3)
```

**Recency** decays from the last call:
- Within the last hour → 1.0
- Within the last day → ~0.9
- Within the last week → ~0.7
- Within the last month → ~0.4
- Within the last year → ~0.1

**Frequency** is normalized against a baseline of 100 calls, capped at 1.0.

**Warmth levels:** `hot` ≥ 0.7, `warm` ≥ 0.3, `cold` < 0.3.

## Reset on Code Change

When a Block's implementation file hash changes, the wrapper detects it and logs a `block.updated` event. The measurement window resets -- vitals start fresh from the new version. Old behavioral data from a different implementation is discarded.

## Observed Edges

**Observed callees** are mined from `tool.call` log events. They show which tool Blocks the reasoning actually uses at runtime.

**Observed callers** require a cross-block scan (only during `aglet stats`, not per-run). They show which Blocks depend on this one.

`aglet validate` compares observed callees against declared `tools`:
- **Undeclared dependency** -- appears in observed_callees but not in `tools`.
- **Dead declared tool** -- in `tools` but never observed after 20+ calls.

## Sink

Each domain can configure where runtime data is forwarded via the `sink` field in `domain.yaml`:

```yaml
aglet:
  sink: local                    # default — .aglet/ only
  # sink: https://ams.example.com  # forward to remote endpoint
```

The wrapper always writes locally to `.aglet/` first. If `sink` is a URL, the log entry is also forwarded in a fire-and-forget goroutine. Blocks can override the domain sink with their own `sink` field in `block.yaml`.

## aglet stats

Read vitals from `.aglet/` and optionally recompute from logs:

```bash
aglet stats EmailClassifier          # single block
aglet stats --domain intelligence    # domain rollup
aglet stats --project                # all blocks
aglet stats EmailClassifier --json   # JSON output
```

## aglet snapshot

Commit vitals files in all `.aglet/` repos:

```bash
aglet snapshot
```

Creates a git commit in each domain's `.aglet/` repo with a reference to the main repo's current HEAD. Use this after significant runs to capture a behavioral checkpoint.

See the [CLI Reference](/cli/) for full flag documentation.
