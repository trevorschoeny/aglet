---
title: Guardrails
---

# Guardrails

These rules protect the integrity of the Aglet paradigm. They are not conventions -- they are constraints.

## Structural Constraints

### 1. Surfaces Cannot Contain Other Surfaces

A Surface is a top-level executable. Nesting Surfaces would recreate monolith architecture with extra steps. If you need a second frontend, create a sibling Surface -- never a child.

`aglet validate` checks for nested `surface.yaml` files and reports a violation if found.

### 2. Blocks Cannot Depend on Surfaces

The dependency arrow is one-way: Surfaces consume Blocks (via the contract in `surface.yaml`), never the reverse. If a Block's `calls` field references a Surface, that's a validation error. The Block graph must remain a pure, self-contained functional web that exists independently of any UI.

### 3. Only Process and Reasoning Blocks Are Externally Executable

- **Process Blocks** are executed by their language runner (e.g., `python3`, `go run`).
- **Reasoning Blocks** are executed by the `aglet-reason` runner.
- **Embedded Blocks** are internal to Surfaces and cannot be executed externally -- the `aglet` CLI rejects them with a clear error.
- **Surfaces** are started by their own build/runtime tools (Vite, Tauri, etc.).

This separation is physical, not just conventional.

### 4. Blocks Are the Default Unit of Logic

Surfaces exist specifically for when the system needs a stateful, interactive interface facing users. If you're building an API, a pipeline, a webhook handler, a cron job, or a CLI tool -- those are Block graphs, not Surfaces. Surfaces are the endpoints you attach for interaction. They don't change anything about the core of what the program does functionally.

### 5. The `tools` Field Can Only Reference Process or Reasoning Blocks

Embedded Blocks are internal to Surfaces and cannot be invoked as tools during reasoning. The `aglet-reason` runner can only spawn Blocks that follow the stdin/stdout protocol. If a reasoning Block needs a capability that currently exists as an embedded Block, promote it to a process Block first.

`aglet validate` checks that every entry in `tools` references an existing Block with `runtime: process` or `runtime: reasoning`, and reports an error for embedded Block references.

## The Sync Check System

Run `aglet validate` after every significant change. The CLI handles deterministic checks automatically and auto-fixes what it can. Beyond deterministic checks, there are judgment-based checks that require human or reasoning Block review.

### Deterministic Checks (Handled by `aglet validate`)

**All units (Blocks, Surfaces, Components, Domains):**

- UUID is unique and has the correct prefix (`b-`, `s-`, `c-`, `d-`)
- `name` in the identity YAML matches the folder name
- `domain` field references a real domain with a `domain.yaml`
- `intent.md` exists in the unit directory

**Process and Embedded Blocks:**

- `runtime` is one of: `process`, `embedded`, `reasoning`
- `impl` field (or a `main.*` file) exists
- `schema.in` and `schema.out` are present in `block.yaml`
- Every `calls` entry references an existing Block
- No circular dependencies in the `calls` graph

**Reasoning Blocks:**

- `model` is set or inheritable from root domain defaults
- `prompt` file (default `prompt.md`) exists
- `tools` references only existing Blocks with `runtime: process` or `runtime: reasoning`
- No `main.*` file exists
- If `provider` is set, it matches a provider in root `domain.yaml`

**Surfaces:**

- `entry` file exists
- No nested Surfaces
- Contract dependencies reference real Blocks or pipeline start Blocks

**Components:**

- `consumes` entries exist in the parent Surface's contract
- Bidirectional traceability: contract callers match component consumes (and vice versa)

**Domains:**

- `parent` references an existing domain

### Auto-Fix Behavior

`aglet validate` fixes what it can and reports what it can't. Fixable errors are resolved in-place.

| Error | Auto-Fix |
|-------|----------|
| Name/folder mismatch | Updates YAML `name` to match folder (folder is source of truth) |
| Missing `intent.md` | Creates stub with `# Name` and a TODO placeholder |
| Missing `prompt.md` (reasoning) | Creates stub with `# Name` and a TODO placeholder |
| Contract lists caller, component missing from `consumes` | Adds dependency to component's `consumes` |
| Component consumes, contract missing caller | Adds component to contract's `callers` |
| Domain parent references non-existent domain | Infers parent from filesystem nesting |

**Not auto-fixable** (requires human design decisions):

- Missing schemas (`schema.in`, `schema.out`)
- Circular dependencies
- References to non-existent Blocks (in `calls`, `tools`, contract)
- Invalid provider references
- Missing implementation files

### Judgment-Based Checks (Future `--deep`)

These require human or AI review and are not yet automated:

- `intent.md` accurately describes what the unit actually does
- `intent.md` is comprehensive enough to fully convey the unit's purpose
- `schema.in` matches what the implementation actually reads
- `schema.out` matches what the implementation actually writes
- Every Block in `calls` has a compatible `schema.in` for what this Block sends
- Process Block implementations follow the In/Transform/Out convention
- Embedded Block implementations export a pure function with no state dependencies
- `prompt.md` fully conveys the reasoning framework and constraints
- The Surface's contract is up to date with every external data dependency
- Transformation logic is in embedded Blocks, not in Components directly

## Development Flow

### Backend-First (Block Pipeline First)

1. **Go breadth-first across the Block graph.** Sketch the full pipeline before going deep on any single Block. Create `block.yaml` and `intent.md` for each Block first -- names, descriptions, edges, and a paragraph of intent.
2. **Split when a Block does two things.** The test: can you describe this Block's job in one sentence without "and"? If you need "and," that's two Blocks.
3. **Don't go too granular too early.** If splitting would create something trivially small or speculative, add a note in "Future Capabilities" and move on.

### Surface-First (Frontend Experience First)

1. **Start with the intent and the contract.** Write the Surface's `intent.md`, then populate the contract in `surface.yaml` in real time as you build Components.
2. **Sketch the Component domains breadth-first.** Lay out the domains and top-level Components before building any deeply.
3. **Use mock data to simulate the backend.** Build the Surface against mock data shaped like the contract declares. When the backend is built, it implements the contract and the frontend doesn't change.
4. **The contract becomes the backend spec.** Each dependency entry maps naturally to one or more Blocks.
5. **Identify embedded Blocks as you build.** When you find transformation logic inside a Component, pull it into an embedded Block immediately.

### Both Approaches

Design and code co-evolve. Once the breadth-first sketch exists, start implementing. As you implement, update the design layer (intents, identity files, schemas, contract) in the same pass as the code.

## Creating New Units

### New Block

1. Create the directory inside the appropriate domain folder.
2. Write `block.yaml` with a generated typed UUID (`b-` prefix), name, description, domain, role, runtime, impl, calls, and schemas.
3. Write `intent.md` with at minimum a summary paragraph and "Why This Exists."
4. Write `main.*` with the In/Transform/Out structure and a stubbed Block function.

A Block is born complete or not at all.

### New Reasoning Block

1. Create the directory inside the appropriate domain folder.
2. Write `block.yaml` with `runtime: reasoning`, model (or inherit from domain), prompt pointer, tools, calls, and schemas. The `schema.out` is especially important -- the runner uses it to enforce structured output.
3. Write `intent.md` explaining why this is a reasoning task rather than a deterministic one.
4. Write `prompt.md` -- the system prompt that implements the reasoning.
5. Identify any Blocks that should be available as tools and list them in `tools`.

No `main.*` file.

### New Surface

1. Create the directory inside the appropriate domain.
2. Write `surface.yaml` with a generated UUID (`s-` prefix), name, description, domain, version (`0.1.0`), framework, bundler, dev config, entry point, and an initial `contract:` section.
3. Write `intent.md` -- comprehensive founding vision for the frontend experience.
4. Write `main.*` -- the entry point that bootstraps the application.
5. Initialize framework project files (`package.json`, `vite.config.ts`, etc.) as needed.
6. Plan the Component domain structure.

### New Component

1. Create the directory inside the appropriate domain within the Surface.
2. Write `component.yaml` with a generated UUID (`c-` prefix), name, description, domain, role, permissions, and analytics config.
3. Write `intent.md`.
4. Write the implementation file, named after the Component.
5. If the Component needs server data, immediately add a contract entry to `surface.yaml` (with this Component in `callers`) and add the dependency name to the Component's `consumes` field.

### New Domain

1. Create the directory inside its parent domain.
2. Write `domain.yaml` with a generated UUID (`d-` prefix), name, parent, and any default overrides.
3. Write `intent.md` -- the domain's founding document.
4. Verify the folder location matches the declared parent chain.

## Splitting a Block

When a Block needs to split into two:

1. Create the new Block directory with all files (`block.yaml`, `intent.md`, `main.*`).
2. Redistribute the intent -- each Block gets the portion of the original reasoning that applies to it.
3. Update `schema.in` / `schema.out` in both Blocks' `block.yaml` to reflect the new data boundary.
4. Update `calls` in both Blocks -- the original Block probably now calls the new one.
5. Update any other Blocks that referenced the original if their edge should now point to the new Block.
6. Update implementations to match.
