import type { AgletOptions, AgletInstance, InteractionEvent } from "./types";

// Default flush interval: 5 minutes (300 seconds)
const DEFAULT_FLUSH_INTERVAL = 300;

// Endpoint for flushing interaction events to the domain listener.
// The listener appends these to the surface's logs.jsonl.
const EVENTS_ENDPOINT = "/_aglet/events";

// --- Global auto-tracking state ---
// Auto-tracking attaches global DOM listeners once (on first createAglet call)
// and shares the event buffer across all instances. This avoids duplicate
// listeners when multiple components each create their own aglet instance.
let globalBuffer: InteractionEvent[] = [];
let globalSurface = "";
let globalBaseUrl = "";
let globalFlushTimer: ReturnType<typeof setInterval> | null = null;
let globalListenersAttached = false;
let instanceCount = 0;

/**
 * Reads SDK config injected by the domain listener into the HTML.
 * The listener reads surface.yaml and injects:
 *   <script>window.__AGLET__ = {surface: "Dashboard", flushInterval: 300, ...}</script>
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
 * Push an event into the global buffer.
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
 * Uses sendBeacon (reliable during page unload) with fetch fallback.
 */
function globalFlush(): void {
  if (globalBuffer.length === 0) return;

  const batch = globalBuffer;
  globalBuffer = [];

  const payload = JSON.stringify(batch);
  const url = globalBaseUrl + EVENTS_ENDPOINT;

  if (typeof navigator !== "undefined" && navigator.sendBeacon) {
    navigator.sendBeacon(url, payload);
  } else if (typeof fetch !== "undefined") {
    fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: payload,
    }).catch(() => {
      // Silent failure — observability should never break the app
    });
  }
}

/**
 * Truncate text to a max length for log readability.
 */
function truncate(text: string, max: number): string {
  if (text.length <= max) return text;
  return text.slice(0, max) + "…";
}

/**
 * Extract a human-readable label from a clicked element.
 * Tries: aria-label, textContent, id, tag+class.
 */
function describeElement(el: Element): Record<string, string> {
  const info: Record<string, string> = { tag: el.tagName.toLowerCase() };

  if (el.id) info.id = el.id;

  const ariaLabel = el.getAttribute("aria-label");
  if (ariaLabel) {
    info.label = truncate(ariaLabel, 80);
  } else if (el.textContent) {
    const text = el.textContent.trim();
    if (text && text.length < 120) {
      info.text = truncate(text, 80);
    }
  }

  // Check for data-aglet-component attribute for component attribution
  const agletComponent = el.closest("[data-aglet]");
  if (agletComponent) {
    info.component = agletComponent.getAttribute("data-aglet") ?? "";
  }

  return info;
}

/**
 * Attach global DOM listeners for auto-tracking.
 * Called once on first createAglet instance. Captures:
 * - Clicks on interactive elements (buttons, links, inputs)
 * - Form submissions
 * - Navigation (popstate, i.e., back/forward)
 */
function attachGlobalListeners(): void {
  if (globalListenersAttached) return;
  if (typeof document === "undefined") return;
  globalListenersAttached = true;

  // --- Click tracking ---
  // Captures clicks on interactive elements: buttons, links, inputs,
  // and anything with role="button". Ignores passive elements (divs, spans)
  // to reduce noise.
  document.addEventListener(
    "click",
    (e: MouseEvent) => {
      const target = e.target as Element | null;
      if (!target) return;

      // Walk up to find the nearest interactive element
      const interactive = target.closest(
        "a, button, input, select, textarea, [role='button'], [onclick]",
      );
      if (!interactive) return;

      const detail = describeElement(interactive);

      // For links, include the href
      if (interactive.tagName === "A") {
        const href = interactive.getAttribute("href");
        if (href) detail.href = href;
      }

      pushEvent("click", detail.component ?? "", detail);
    },
    { capture: true, passive: true },
  );

  // --- Form submission tracking ---
  document.addEventListener(
    "submit",
    (e: Event) => {
      const form = e.target as HTMLFormElement | null;
      if (!form) return;

      const detail: Record<string, string> = { tag: "form" };
      if (form.id) detail.id = form.id;
      if (form.action) detail.action_url = form.action;
      if (form.method) detail.method = form.method;

      const agletComponent = form.closest("[data-aglet]");
      const caller = agletComponent?.getAttribute("data-aglet") ?? "";

      pushEvent("form_submit", caller, detail);
    },
    { capture: true, passive: true },
  );

  // --- Navigation tracking (back/forward) ---
  window.addEventListener("popstate", () => {
    pushEvent("navigate", "", {
      url: window.location.pathname,
      type: "popstate",
    });
  });

  // --- Page visibility tracking (tab switch / minimize) ---
  document.addEventListener("visibilitychange", () => {
    pushEvent("visibility", "", {
      state: document.visibilityState,
    });
  });

  // --- Flush on page unload ---
  window.addEventListener("beforeunload", globalFlush);
}

/**
 * Start the global flush timer if not already running.
 */
function startGlobalFlushTimer(intervalSec: number): void {
  if (globalFlushTimer !== null) return;
  if (typeof setInterval === "undefined") return;
  if (intervalSec <= 0) return;
  globalFlushTimer = setInterval(globalFlush, intervalSec * 1000);
}

/**
 * Stop global auto-tracking and clean up when no instances remain.
 */
function stopGlobalTracking(): void {
  if (instanceCount > 0) return;

  if (globalFlushTimer !== null) {
    clearInterval(globalFlushTimer);
    globalFlushTimer = null;
  }

  // Note: we don't remove the DOM listeners — they're passive and harmless.
  // Removing them would require storing references, adding complexity for
  // a case (all instances destroyed) that rarely happens in practice.
}

/**
 * createAglet creates an SDK instance bound to a specific component.
 *
 * Each component in a Surface should create its own instance so that
 * contract calls carry the correct caller identity. Interaction events
 * are tracked automatically at the page level (clicks, form submissions,
 * navigation) — no manual track() calls needed for basic observability.
 *
 * For component-level attribution of auto-tracked events, add
 * data-aglet="ComponentName" to the component's root DOM element.
 *
 * Usage:
 *   const aglet = createAglet('FeedbackPanel')
 *   const result = await aglet.call('Sentiment', { text: 'hello' })
 *   // Clicks, form submissions, navigation are tracked automatically
 *   // Use track() for custom events specific to your component:
 *   aglet.track('custom_action', { some: 'detail' })
 */
export function createAglet(
  component: string,
  options: AgletOptions = {},
): AgletInstance {
  // Merge config: explicit options > injected from surface.yaml > defaults.
  const injected = getInjectedConfig();
  const surface = options.surface ?? injected.surface ?? "";
  const baseUrl = options.baseUrl ?? injected.baseUrl ?? "";
  const flushIntervalSec =
    options.flushInterval ?? injected.flushInterval ?? DEFAULT_FLUSH_INTERVAL;
  const trackEnabled =
    options.trackInteractions ?? injected.trackInteractions ?? true;

  // Set global state from first instance
  if (instanceCount === 0) {
    globalSurface = surface;
    globalBaseUrl = baseUrl;
  }
  instanceCount++;

  // --- Auto-tracking setup ---
  // Attach global DOM listeners (once) and start the flush timer.
  // Also log a component_mount event for this specific component.
  if (trackEnabled && typeof window !== "undefined") {
    attachGlobalListeners();
    startGlobalFlushTimer(flushIntervalSec);
    pushEvent("component_mount", component, {});
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
      pushEvent(action, component, detail);
    },

    flush: globalFlush,

    destroy(): void {
      // Log component unmount
      if (trackEnabled) {
        pushEvent("component_unmount", component, {});
      }

      // Flush remaining events
      globalFlush();

      // Decrement instance count and clean up if last instance
      instanceCount = Math.max(0, instanceCount - 1);
      stopGlobalTracking();
    },
  };
}
