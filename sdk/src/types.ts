// --- SDK Configuration ---

/** Options passed to createAglet. */
export interface AgletOptions {
  /** Surface name (from surface.yaml). Used in event logs. */
  surface?: string;

  /** Base URL for contract endpoints. Defaults to '' (relative, same origin). */
  baseUrl?: string;

  /** Flush interval for interaction events, in seconds. Default: 300 (5 min). */
  flushInterval?: number;
}

// --- Event Types ---

/** A client-side event buffered by the SDK and flushed to the domain listener. */
export interface AgletEvent {
  /** Event type — "mount", "unmount", "track", or any custom action. */
  event: string;

  /** ISO 8601 timestamp when the event occurred. */
  timestamp: string;

  /** Component that produced this event. */
  caller: string;

  /** Surface this component belongs to. */
  surface: string;

  /** What kind of event — "component_mount", "component_unmount", or custom. */
  action: string;

  /** Optional additional data about the event. */
  detail?: Record<string, unknown>;
}

// --- Aglet Instance ---

/** The object returned by createAglet — one per component. */
export interface AgletInstance {
  /** The component name this instance is bound to. */
  readonly component: string;

  /**
   * Log that this component has mounted (appeared on screen).
   * Call this when the component initializes.
   */
  mount: () => void;

  /**
   * Log that this component has unmounted (removed from screen).
   * Call this when the component is being torn down.
   */
  unmount: () => void;

  /**
   * Call a contract endpoint on the domain listener.
   * Adds X-Aglet-Caller and X-Aglet-Surface headers automatically.
   * Returns the parsed JSON response.
   */
  call: <TInput = unknown, TOutput = unknown>(
    contract: string,
    input: TInput,
  ) => Promise<TOutput>;

  /**
   * Track a custom event for this component.
   * Events are buffered and flushed periodically + on page unload.
   */
  track: (action: string, detail?: Record<string, unknown>) => void;

  /**
   * Manually flush all buffered events to the domain listener.
   * Called automatically on interval and beforeunload.
   */
  flush: () => void;

  /**
   * Tear down this instance — flush remaining events and clean up.
   * Does NOT log unmount — call unmount() explicitly before this if needed.
   */
  destroy: () => void;
}
