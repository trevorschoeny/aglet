---
title: Future Features
---

# Future Features

Capabilities that are designed but not yet implemented. These represent where Aglet is heading.

## Database Connectors — Future Enhancements

Basic store configuration is implemented: `stores` in `domain.yaml` declares database connections, and the wrapper injects `AGLET_STORE_{NAME}` environment variables into process Blocks at runtime. Developers use their own database libraries. See the [Domains](/spec/domains) specification for details.

Future enhancements under consideration:

**Query observability.** The AML could track query counts, latency, and error rates per store per Block — similar to how it tracks tool calls for reasoning Blocks. This would require a lightweight query proxy or instrumentation layer.

**Connection pooling.** For long-running domain listeners, a shared connection pool per store (e.g., PgBouncer-style) managed by the domain listener process rather than individual Block executions.

**Block-level store declarations.** An optional `stores` field in `block.yaml` for explicit dependency declaration — making store dependencies visible to agents and `aglet validate` without requiring it.

## Secrets Management

Seamless secrets handling that works identically in dev and prod. No environment variable juggling, no `.env` files that drift between environments.

The vision: secrets declared in domain.yaml by name, resolved at runtime by the wrapper from a configured backend. In dev, the backend is the OS keychain (macOS Keychain, Linux secret-service). In prod, the backend is a vault (AWS Secrets Manager, HashiCorp Vault, etc.). The developer never changes code or config between environments — only the secret backend changes.

```yaml
# domain.yaml
secrets:
  backend: keychain            # dev: OS keychain
  # backend: aws-secrets       # prod: AWS Secrets Manager
  # backend: vault             # prod: HashiCorp Vault

providers:
  anthropic:
    secret: ANTHROPIC_API_KEY  # resolved from whichever backend is configured

stores:
  readings:
    type: postgres
    secret: DATABASE_URL       # same — resolved at runtime
```

The wrapper injects resolved secrets into the block's environment before execution. The block never knows where the secret came from. Secrets never appear in YAML files, logs, or vitals.

## User Authentication & Authorization

Auth for the end users of Aglet applications. Two layers:

**Frontend auth** can use existing framework solutions (NextAuth, Clerk, Auth0). Surfaces already run on standard frameworks — no need to reinvent this.

**Backend auth across decentralized domains** is the harder problem. In a traditional monolith, one server checks auth. In Aglet, requests flow through domain listeners and block wrappers across potentially separate services. The auth context needs to propagate through the pipeline.

The likely approach: an `auth` section in domain.yaml that declares an auth provider. The domain listener validates tokens on incoming requests and attaches an auth context. Block wrappers forward this context through the pipeline. Blocks can read auth context from a standard header or environment variable — they don't handle auth themselves.

```yaml
# domain.yaml
auth:
  provider: jwt
  secret: JWT_SIGNING_KEY
  propagate: true              # forward auth context through calls edges
```

Individual blocks could declare auth requirements:

```yaml
# block.yaml
auth:
  required: true
  roles: [admin, editor]       # RBAC at the block level
```

## Developer Authorization & Access Control

Fine-grained access control for who can view and modify different parts of an Aglet project. Declared in YAML, enforced by tooling and the AMS.

```yaml
# domain.yaml
access:
  roles:
    admin:
      - implementation: write
      - intent: write
      - config: write
      - logs: read
      - vitals: read
    developer:
      - implementation: write
      - intent: write
      - config: read
      - logs: read
      - vitals: read
    analyst:
      - implementation: none
      - intent: read
      - config: none
      - logs: read
      - vitals: read
    observer:
      - logs: read
      - vitals: read

  users:
    trevor: admin
    alice: developer
    bob: analyst
```

This maps directly to the implementation-vs-wrapper distinction: the implementation (source code) and the semantic layer (vitals, logs) are separate concerns with separate access levels. An analyst can read all behavioral data without seeing any code. A developer can modify code but can't change domain config. The AMS would enforce these in the dashboard; the CLI could enforce them locally via the `.aglet/` git repo permissions.

## Custom Vitals Metrics

Developers declare custom metrics in block.yaml that the wrapper tracks alongside the built-in vitals. A reasoning block could track `avg_confidence` from its output schema. A process block could track `avg_output_size_kb`. The wrapper reads the metric definitions and computes them incrementally, same as the built-in vitals.

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

The AMS would also be the enforcement layer for developer access control — the YAML declarations define the policy, the AMS enforces it.

## Production Domain Listeners

Domain listeners deployable as production services — same binary as dev, with production-grade features: graceful shutdown, health checks, connection pooling, TLS, and horizontal scaling behind a load balancer.

## Cross-Domain Peer Discovery

Automatic peer discovery between domains using DNS or a lightweight registry, replacing manual `peers:` configuration. Domains announce themselves and discover neighbors without hardcoded URLs.

