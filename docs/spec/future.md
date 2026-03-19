---
title: Future Features
---

# Future Features

Capabilities that are designed but not yet implemented. These represent where Aglet is heading.

## Custom Vitals Metrics

Developers declare custom metrics in block.yaml that the wrapper tracks alongside the built-in vitals. A reasoning Block could track `avg_confidence` from its output schema. A process Block could track `avg_output_size_kb`. The wrapper reads the metric definitions and computes them incrementally, same as the built-in vitals.

```yaml
# block.yaml
vitals:
  custom:
    avg_confidence:
      source: output.confidence
      type: average
    max_latency_ms:
      source: duration_ms
      type: max
```

## YAML-Driven SDK Observability

Surface and component observability config — what to track, flush intervals, interaction types — declared in surface.yaml and component.yaml. Automatically injected into the client SDK via the domain listener. No code changes needed to adjust tracking behavior.

```yaml
# component.yaml
observe:
  mount: true
  interactions: true
  events: [click, submit]
```

## Warmth-Based Wrapper Cooldown

Hot blocks (high warmth score) keep their wrappers alive between calls for zero cold-start latency. Cold blocks are fully serverless. The cooldown period is configurable in the observe contract or domain config.

## WASM Compilation

`aglet build` compiles process blocks to WebAssembly modules for portable, near-instant execution. The wrapper becomes a WASM host instead of a subprocess spawner. Blocks become single `.wasm` files that run anywhere — server, edge, browser.

## Aglet Management System (AMS)

A hosted dashboard that aggregates vitals, logs, and behavioral data across domains, environments, and teams. The `sink` config in domain.yaml points to the AMS endpoint. What GitHub is to git — the collaboration and visibility layer on top of the protocol.

## Storage Integration

First-class storage primitives for Aglet projects — persistent data stores that blocks and surfaces can read from and write to, declared in YAML and managed by the protocol. Replaces ad-hoc JSON file stores with something the AML can observe.

## Production Domain Listeners

Domain listeners deployable as production services — same binary as dev, with production-grade features: graceful shutdown, health checks, connection pooling, TLS, and horizontal scaling behind a load balancer.

## Cross-Domain Peer Discovery

Automatic peer discovery between domains using DNS or a lightweight registry, replacing manual `peers:` configuration. Domains announce themselves and discover neighbors without hardcoded URLs.
