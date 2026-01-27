// Utility function for debouncing function calls.

export interface DebouncedFunction<T extends (...args: unknown[]) => unknown> {
  (...args: Parameters<T>): void;
  /** Immediately execute the pending function if one is scheduled. */
  flush(): void;
  /** Cancel any pending execution without calling the function. */
  cancel(): void;
  /** Returns true if there's a pending execution. */
  pending(): boolean;
}

/**
 * Creates a debounced version of a function that only calls it after
 * the specified delay has elapsed since the last call.
 *
 * The returned function has additional methods:
 * - flush(): Immediately execute the pending function
 * - cancel(): Cancel the pending execution
 * - pending(): Check if there's a pending execution
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function debounce<T extends (...args: any[]) => any>(fn: T, delayMs: number): DebouncedFunction<T> {
  let timeoutId: ReturnType<typeof setTimeout> | null = null;
  let lastArgs: Parameters<T> | null = null;

  const debounced = (...args: Parameters<T>) => {
    lastArgs = args;
    if (timeoutId !== null) {
      clearTimeout(timeoutId);
    }
    timeoutId = setTimeout(() => {
      timeoutId = null;
      lastArgs = null;
      fn(...args);
    }, delayMs);
  };

  debounced.flush = () => {
    if (timeoutId !== null && lastArgs !== null) {
      clearTimeout(timeoutId);
      const args = lastArgs;
      timeoutId = null;
      lastArgs = null;
      fn(...args);
    }
  };

  debounced.cancel = () => {
    if (timeoutId !== null) {
      clearTimeout(timeoutId);
      timeoutId = null;
      lastArgs = null;
    }
  };

  debounced.pending = () => timeoutId !== null;

  return debounced;
}
