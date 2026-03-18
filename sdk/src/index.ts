// @aglet/sdk — Client-side SDK for Aglet surfaces
//
// Two jobs:
// 1. Wrap contract calls (adds X-Aglet-Caller header, server handles logging)
// 2. Buffer and flush client-side interaction events (periodic + beforeunload)
//
// Usage:
//   import { createAglet } from '@aglet/sdk'
//   const aglet = createAglet('FeedbackPanel', { surface: 'Dashboard' })
//   const result = await aglet.call('Sentiment', { text: 'hello' })
//   aglet.track('button_click', { action: 'submit' })

export { createAglet } from "./aglet";
export type {
  AgletOptions,
  AgletInstance,
  InteractionEvent,
} from "./types";
