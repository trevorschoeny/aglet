import type { AgletOptions, AgletInstance, AgletEvent } from "./types";

// Default flush interval: 5 minutes (300 seconds)
const DEFAULT_FLUSH_INTERVAL = 300;

// Endpoint for flushing events to the domain listener.
// The listener appends these to the surface's logs.jsonl.
const EVENTS_ENDPOINT = "/_aglet/events";

// --- Shared global state ---
// All aglet instances share one buffer and one flush timer.
// This keeps memory usage flat regardless of how many components
// create instances — they all write to the same queue.
let globalBuffer: AgletEvent[] = [];
let globalSurface = "";
let globalBaseUrl = "";
let globalFlushTimer: ReturnType<typeof setInterval> | null = null;
let instanceCount = 0;

/**
 * Reads SDK config injected by the domain listener into the HTML.
 * The listener reads surface.yaml and injects:
 *   <script>window.__AGLET__ = {surface: "Dashboard", flushInterval: 300}</script>
 *
 * Returns the config object, or an empty object if not found.
 */
function getInjectedConfig(): Partial<AgletOptions> {
  if (typeof window === "undefined") return {};
  const config = (window as unknown as Record<string, unknown>).__AGLET__;
  if (config && typeof config === "object") {
    return config as Partial<AgletOptions>;
  }
  return {};
}

/**
 * Push an event into the shared buffer.
 */
function pushEvent(
  action: string,
  caller: string,
  detail?: Record<string, unknown>,
): void {
  globalBuffer.push({
    event: "interaction",
    timestamp: new Date().toISOString(),
    caller,
    surface: globalSurface,
    action,
    detail,
  });
}

/**
 * Flush all buffered events to the domain listener.
 * Uses sendBeacon for reliability during page unload, with fetch as fallback.
 */
function globalFlush(): void {
  if (globalBuffer.length === 0) return;

  // Drain the buffer atomically
  const batch = globalBuffer;
  globalBuffer = [];

  const payload = JSON.stringify(batch);
  const url = globalBaseUrl + EVENTS_ENDPOINT;

  // sendBeacon is reliable during page unload — the browser guarantees delivery
  // even if the page is closing. fetch may be cancelled during unload.
  if (typeof navigator !== "undefined" && navigator.sendBeacon) {
    navigator.sendBeacon(url, payload);
  } else if (typeof fetch !== "undefined") {
    fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: payload,
    }).catch(() => {
      // Silent failure — observability should never break the app.
      // If the domain listener isn't running (production without log endpoint),
      // events are simply dropped.
    });
  }
}

/**
 * Start the shared flush timer if not already running.
 */
function startFlushTimer(intervalSec: number): void {
  if (globalFlushTimer !== null) return;
  if (typeof setInterval === "undefined") return;
  if (intervalSec <= 0) return;
  globalFlushTimer = setInterval(globalFlush, intervalSec * 1000);
}

/**
 * Register the beforeunload handler (once) so events are flushed
 * when the user closes or navigates away from the page.
 */
let unloadRegistered = false;
function registerUnloadFlush(): void {
  if (unloadRegistered) return;
  if (typeof window === "undefined") return;
  unloadRegistered = true;
  window.addEventListener("beforeunload", globalFlush);
}

/**
 * Clean up global resources when no instances remain.
 */
function cleanupIfEmpty(): void {
  if (instanceCount > 0) return;
  if (globalFlushTimer !== null) {
    clearInterval(globalFlushTimer);
    globalFlushTimer = null;
  }
}

/**
 * createAglet creates an SDK instance bound to a specific component.
 *
 * Each component in a Surface creates its own instance. The instance
 * provides mount/unmount lifecycle logging, contract calls with automatic
 * caller headers, and custom event tracking.
 *
 * All instances share a single event buffer and flush timer.
 * No DOM interaction — mount/unmount are explicit calls in your component code.
 *
 * Usage (React example):
 *
 *   import { createAglet } from '@aglet/sdk'
 *
 *   function FeedbackPanel() {
 *     useEffect(() => {
 *       const aglet = createAglet('FeedbackPanel')
 *       aglet.mount()
 *       return () => {
 *         aglet.unmount()
 *         aglet.destroy()
 *       }
 *     }, [])
 *   }
 */
export function createAglet(
  component: string,
  options: AgletOptions = {},
): AgletInstance {
  // Merge config: explicit options > injected from surface.yaml > defaults
  const injected = getInjectedConfig();
  const surface = options.surface ?? injected.surface ?? "";
  const baseUrl = options.baseUrl ?? injected.baseUrl ?? "";
  const flushIntervalSec =
    options.flushInterval ?? injected.flushInterval ?? DEFAULT_FLUSH_INTERVAL;

  // Initialize global state from the first instance
  if (instanceCount === 0) {
    globalSurface = surface;
    globalBaseUrl = baseUrl;
  }
  instanceCount++;

  // Start the flush timer and register the unload handler
  startFlushTimer(flushIntervalSec);
  registerUnloadFlush();

  return {
    get component() {
      return component;
    },

    // --- Lifecycle ---

    mount(): void {
      pushEvent("component_mount", component);
    },

    unmount(): void {
      pushEvent("component_unmount", component);
    },

    // --- Contract calls ---

    async call<TInput = unknown, TOutput = unknown>(
      contract: string,
      input: TInput,
    ): Promise<TOutput> {
      // POST to /contract/<name> on the domain listener.
      // X-Aglet-Caller and X-Aglet-Surface headers let the block wrapper
      // log which component triggered the call and which surface it belongs to.
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

    // --- Custom tracking ---

    track(action: string, detail?: Record<string, unknown>): void {
      pushEvent(action, component, detail);
    },

    // --- Buffer management ---

    flush: globalFlush,

    destroy(): void {
      // Flush any remaining events
      globalFlush();

      // Decrement and clean up if last instance
      instanceCount = Math.max(0, instanceCount - 1);
      cleanupIfEmpty();
    },
  };
}
