// --- SDK Configuration ---

/** Options passed to createAglet. */
export interface AgletOptions {
  /** Surface name (from surface.yaml). Used in interaction event logs. */
  surface?: string;

  /** Base URL for contract endpoints. Defaults to '' (relative, same origin). */
  baseUrl?: string;

  /** Flush interval for interaction events, in seconds. Default: 300 (5 min). */
  flushInterval?: number;

  /** Whether to track client-side interactions via aglet.track(). Default: true. */
  trackInteractions?: boolean;
}

// --- Event Types ---

/** A client-side interaction event buffered by the SDK. */
export interface InteractionEvent {
  /** Event type — always "interaction" for client-side events. */
  event: "interaction";

  /** ISO 8601 timestamp when the interaction occurred. */
  timestamp: string;

  /** Component that produced this event. */
  caller: string;

  /** Surface this component belongs to. */
  surface: string;

  /** What kind of interaction — "click", "navigate", "submit", etc. */
  action: string;

  /** Optional additional data about the interaction. */
  detail?: Record<string, unknown>;
}

// --- Aglet Instance ---

/** The object returned by createAglet — one per component. */
export interface AgletInstance {
  /** The component name this instance is bound to. */
  readonly component: string;

  /**
   * Call a contract endpoint on the domain listener.
   * Adds X-Aglet-Caller header automatically.
   * Returns the parsed JSON response.
   */
  call: <TInput = unknown, TOutput = unknown>(
    contract: string,
    input: TInput,
  ) => Promise<TOutput>;

  /**
   * Track a client-side interaction event.
   * Events are buffered and flushed periodically + on page unload.
   */
  track: (action: string, detail?: Record<string, unknown>) => void;

  /**
   * Manually flush all buffered interaction events.
   * Called automatically on interval and beforeunload.
   */
  flush: () => void;

  /**
   * Tear down this instance — flush remaining events and stop the timer.
   * Call this when the component unmounts.
   */
  destroy: () => void;
}
