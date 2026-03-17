---
title: Intent
---

# Intent

`intent.md` is the most important document in every unit. It is the unit's reason for being -- the comprehensive explanation of *why* it exists, what business or UX logic it encodes, and what design decisions shaped it.

Every unit in Aglet has an `intent.md`: Blocks, Components, Domains, and Surfaces.

## Scope and Tone

Domain and Surface intents are broad and visionary. They define founding purpose, sacred constraints, scope boundaries, and design principles for their entire scope.

Block and Component intents are tightly focused. They explain the specific purpose, design decisions, and open questions for that individual unit.

The content carries the distinction -- a root domain intent reads like a founding charter, while a Block intent reads like a focused design rationale -- but they are all `intent.md`, and there is no structural difference in the filename or location.

## Thoroughness

Intents should be thorough. Err on the side of too much detail rather than too little. Every unit, no matter how small, is a meaningful part of the system, and the intent is what makes that meaning explicit.

A large Block handling complex business logic will have a substantial intent document. A small Component (like a single button) will have a shorter one -- but it should still articulate why this specific unit deserves to exist independently and what role it plays in the larger experience.

## Primary Focus: The Why

The primary focus of the intent is the **why** -- the business logic, the purpose, the reasoning behind decisions. It may also include implementation overviews or suggestions for how the code works, but these support the why rather than replace it.

Code explains *how* in full detail. Intent explains *why that how was chosen* and *what purpose it serves*, with enough implementation context to bridge the two.

## Structural Conventions

These sections are conventions, not rigid requirements. The point is that someone -- human or AI -- can read the file and fully understand the unit's reason for existing without reading implementation code.

### Standard Sections (All Units)

```markdown
# UnitName

One-paragraph summary of what this unit does and why.

## Why This Exists

The architectural and business reason this unit needs to exist as
a separate piece. What would break or become unclear if it were
merged into something else?

## Design Decisions

Key choices and their reasoning. What alternatives were considered
and why they were rejected. What tradeoffs were accepted.

## Open Questions

Unresolved design tensions. Park them here rather than embedding
them silently in code. These are active -- they should be revisited.

## Future Capabilities

Things this unit will eventually handle but doesn't yet. Note them
and move on rather than going prematurely deep.
```

### Additional Sections (Domains and Surfaces)

Domain and Surface intents commonly include two additional sections:

```markdown
## Sacred Constraints

Things that must never be compromised, no matter what.

## Who This Serves

The intended audience and their needs.
```

## Examples

### Good Domain Intent

```markdown
# My App

A payment processing system that prioritizes transaction safety
over speed, designed for small merchants who need simple,
auditable checkout flows.

## Why This Exists

Small merchants are underserved by existing payment platforms.
They need a system that is dead simple to integrate, transparent
about what's happening at every step, and built so that no
failure mode can silently eat money.

## Sacred Constraints

- Every transaction must be independently verifiable
- No silent failures -- if something breaks, the merchant knows
- Sub-200ms total pipeline latency for the happy path

## Who This Serves

Small business owners who don't have a dedicated payments team
and need to trust that their checkout just works.

## Design Decisions

- Chose sync-first execution because merchants need immediate
  confirmation, not eventual consistency.
- All Blocks propagate errors by default. Absorb is opt-in and
  must be justified in the Block's intent.

## Future Capabilities

- Multi-currency support (currently USD only)
- Webhook notifications for transaction state changes
```

### Good Block Intent

```markdown
# SentimentAnalyzer

A reasoning Block that classifies text sentiment using LLM judgment.

## Why This Exists

Test Block for validating the Aglet orchestrator's reasoning Block
execution -- LLM API calls, structured output enforcement, and
prompt-as-implementation.

## Design Decisions

- Uses the WordCount tool to demonstrate tool-use loops during
  reasoning. The word count is included in the reasoning output
  to verify the tool was actually called.
- Confidence is a float between 0 and 1 rather than a categorical
  level, giving downstream consumers more flexibility in how they
  threshold decisions.
- Output includes a reasoning field so the classification is
  explainable, not just a label.

## Open Questions

- Should sarcasm detection be a separate pre-processing Block
  or handled within the prompt constraints?
```

## Working With Intent

When making high-level architectural decisions -- which Blocks to create, how to split responsibilities, what the pipeline shape should be -- read the root domain's `intent.md` first. The root intent defines what the system values. If a design choice serves performance but violates a sacred constraint, the constraint wins. If two designs are equivalent, prefer the one more aligned with the intent's stated purpose.

For Surfaces, the Surface's own `intent.md` is the equivalent authority for frontend decisions. If a UX choice conflicts with the Surface intent's vision, the vision wins.

Intent documents are not executable units. They are the north star that every unit within their scope exists in service of.
