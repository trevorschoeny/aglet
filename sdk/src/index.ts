// @aglet/sdk — Client-side observability for Aglet surfaces
//
// Per-component instances with three capabilities:
// 1. Lifecycle: mount() / unmount() — log when components appear and disappear
// 2. Contract calls: call() — fetch a block through the contract endpoint with caller headers
// 3. Custom tracking: track() — log any component-specific event
//
// All events share a single buffer, flushed every 5 min + on page unload.
// No DOM interaction — everything is explicit in your component code.
//
// Usage:
//   import { createAglet } from '@aglet/sdk'
//
//   const aglet = createAglet('FeedbackPanel')
//   aglet.mount()
//   const result = await aglet.call('Sentiment', { text: 'hello' })
//   aglet.track('custom_action', { some: 'detail' })
//   aglet.unmount()
//   aglet.destroy()

export { createAglet } from "./aglet";
export type { AgletOptions, AgletInstance, AgletEvent } from "./types";
