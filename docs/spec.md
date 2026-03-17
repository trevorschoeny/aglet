# The Aglet Specification

> This document is the canonical reference for the Aglet protocol. It is being migrated from an internal specification — full content coming soon.

## Overview

Aglet is a protocol for self-describing computation. Applications are composed of units that carry everything needed to execute them — identity, intent, typed schemas, and implementation — organized within domains and governed by founding intent documents.

The core protocol: any infrastructure that can read a Block's `block.yaml`, send JSON matching the input schema, and receive JSON matching the output schema can host that Block.

## File Types

Aglet has seven file types. Two rules: the YAML filename is the type, and every unit has an `intent.md`.

| File | Purpose | Found in |
|------|---------|----------|
| `block.yaml` | Block identity, schemas, edges | Block directories |
| `surface.yaml` | Surface identity, contract | Surface directories |
| `component.yaml` | Component identity, consumes | Component directories |
| `domain.yaml` | Domain identity, config defaults | Domain directories |
| `intent.md` | Why this unit exists | Every unit directory |
| `prompt.md` | Reasoning Block implementation | Reasoning Block directories |
| `main.*` | Process/Embedded Block implementation | Process/Embedded Block directories |
