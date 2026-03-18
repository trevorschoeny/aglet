import type { AgletOptions, AgletInstance, InteractionEvent } from "./types";

// Default flush interval: 5 minutes (300 seconds)
const DEFAULT_FLUSH_INTERVAL = 300;

// Endpoint for flushing interaction events to the domain listener.
// The listener appends these to the surface's logs.jsonl.
const EVENTS_ENDPOINT = "/_aglet/events";

/**
 * createAglet creates an SDK instance bound to a specific component.
 *
 * Each component in a Surface should create its own instance so that
 * contract calls and interaction events carry the correct caller identity.
 *
 * Usage:
 *   const aglet = createAglet('FeedbackPanel')
 *   const result = await aglet.call('Sentiment', { text: 'hello' })
 *   aglet.track('button_click', { action: 'submit' })
 */
export function createAglet(
  component: string,
  options: AgletOptions = {},
): AgletInstance {
  const surface = options.surface ?? "";
  const baseUrl = options.baseUrl ?? "";
  const flushIntervalSec = options.flushInterval ?? DEFAULT_FLUSH_INTERVAL;
  const trackEnabled = options.trackInteractions ?? true;

  // --- Event buffer ---
  // Interaction events accumulate here and are flushed periodically.
  let buffer: InteractionEvent[] = [];

  // --- Flush logic ---
  // Sends all buffered events to the domain listener via sendBeacon
  // (reliable even during page unload) or falls back to fetch.
  function flush(): void {
    if (buffer.length === 0) return;

    // Drain the buffer atomically
    const batch = buffer;
    buffer = [];

    const payload = JSON.stringify(batch);

    // Prefer sendBeacon — it's fire-and-forget and works during beforeunload.
    // Falls back to fetch for environments without sendBeacon (e.g., Node/SSR).
    const url = baseUrl + EVENTS_ENDPOINT;
    if (typeof navigator !== "undefined" && navigator.sendBeacon) {
      navigator.sendBeacon(url, payload);
    } else if (typeof fetch !== "undefined") {
      // Best-effort — don't await, don't throw on failure
      fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: payload,
      }).catch(() => {
        // Silent failure — observability should never break the app
      });
    }
    // If neither sendBeacon nor fetch exist, events are silently dropped.
    // This is intentional — the SDK should never crash the host application.
  }

  // --- Periodic flush timer ---
  let flushTimer: ReturnType<typeof setInterval> | null = null;
  if (typeof setInterval !== "undefined" && flushIntervalSec > 0) {
    flushTimer = setInterval(flush, flushIntervalSec * 1000);
  }

  // --- Flush on page unload ---
  if (typeof window !== "undefined") {
    window.addEventListener("beforeunload", flush);
  }

  // --- Public API ---
  return {
    get component() {
      return component;
    },

    async call<TInput = unknown, TOutput = unknown>(
      contract: string,
      input: TInput,
    ): Promise<TOutput> {
      // POST to the contract endpoint with the caller header.
      // The domain listener routes this to the block wrapper, which
      // handles all server-side logging (block.start, block.complete,
      // contract.call to surface logs).
      const url = baseUrl + "/contract/" + contract;

      const response = await fetch(url, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Aglet-Caller": component,
          "X-Aglet-Surface": surface,
        },
        body: JSON.stringify(input),
      });

      if (!response.ok) {
        const errorBody = await response.text();
        throw new Error(
          `Contract call '${contract}' failed (${response.status}): ${errorBody}`,
        );
      }

      return response.json() as Promise<TOutput>;
    },

    track(action: string, detail?: Record<string, unknown>): void {
      if (!trackEnabled) return;

      buffer.push({
        event: "interaction",
        timestamp: new Date().toISOString(),
        caller: component,
        surface,
        action,
        detail,
      });
    },

    flush,

    destroy(): void {
      // Flush any remaining events
      flush();

      // Stop the periodic timer
      if (flushTimer !== null) {
        clearInterval(flushTimer);
        flushTimer = null;
      }

      // Remove the beforeunload listener
      if (typeof window !== "undefined") {
        window.removeEventListener("beforeunload", flush);
      }
    },
  };
}
