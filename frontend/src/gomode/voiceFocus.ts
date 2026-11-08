// Publishes cancellable, deadline-bound conversation selections and releases retired focus leases.

import type { UpdateVoiceFocusRequest, UpdateVoiceFocusResponse } from "@sdk/types.gen";

// Focus is advisory to voice setup; a slow host must not hold connection progress.
const PUBLICATION_TIMEOUT_MS = 2000;

export interface VoiceFocusPublisher {
  publish(sessionID: string, nodeID: string | null, isCurrent: () => boolean, signal?: AbortSignal): Promise<void>;
  dispose(): void;
}

export function createVoiceFocusPublisher(
  update: (request: UpdateVoiceFocusRequest, signal: AbortSignal) => Promise<UpdateVoiceFocusResponse>,
  browserSelection: boolean,
): VoiceFocusPublisher {
  let active = true;
  let revision = 0;
  let session = "";
  let version = 0;
  let published: { session: string; version: number } | null = null;
  let pending: AbortController | null = null;

  const release = (lease: { session: string; version: number }) => {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), PUBLICATION_TIMEOUT_MS);
    void cancellable(
      update(
        { "openai/session": lease.session, nodeId: "0", version: lease.version, release: true },
        controller.signal,
      ),
      controller.signal,
    )
      .catch((error: unknown) => console.warn("Could not clear retired voice focus", error))
      .finally(() => clearTimeout(timer));
  };

  return {
    async publish(sessionID, nodeID, isCurrent, signal) {
      if (!sessionID || sessionID.trim() !== sessionID) throw new Error("Invalid conversation ID.");
      if (!active || signal?.aborted || !isCurrent()) return;
      pending?.abort();
      const controller = new AbortController();
      pending = controller;
      const captured = ++revision;
      const current = () => active && captured === revision && !controller.signal.aborted && isCurrent();
      if (session !== sessionID) {
        if (published !== null) release(published);
        published = null;
        session = sessionID;
        version = 0;
      }
      const abort = () => controller.abort(signal?.reason);
      signal?.addEventListener("abort", abort, { once: true });
      const timer = setTimeout(
        () => controller.abort(new DOMException("Voice focus publication timed out.", "TimeoutError")),
        PUBLICATION_TIMEOUT_MS,
      );
      const work = async () => {
        for (let attempt = 0; attempt < 4 && current(); attempt++) {
          const response = await update(
            { "openai/session": sessionID, nodeId: nodeID ?? "0", version, browserSelection },
            controller.signal,
          );
          if (!current()) {
            // The server may have committed before cancellation. A late reply can
            // release only its own version, independently of the next connection.
            if (response.applied) release({ session: sessionID, version: response.version });
            return;
          }
          version = response.version;
          if (response.applied) {
            published = nodeID === null && !browserSelection ? null : { session: sessionID, version: response.version };
            return;
          }
        }
        if (current()) throw new Error("Conversation focus changed concurrently.");
      };
      try {
        await cancellable(work(), controller.signal);
      } catch (error) {
        if (!controller.signal.aborted || controller.signal.reason?.name === "TimeoutError") throw error;
      } finally {
        clearTimeout(timer);
        signal?.removeEventListener("abort", abort);
        if (pending === controller) pending = null;
      }
    },
    dispose() {
      active = false;
      revision++;
      pending?.abort();
      pending = null;
      if (published !== null) release(published);
      published = null;
    },
  };
}

// A callback can outlive cancellation even when fetch honors its signal. Settle
// the owner promptly while its continuation reconciles any late server lease.
function cancellable<T>(work: Promise<T>, signal: AbortSignal): Promise<T> {
  if (signal.aborted) return Promise.reject(signal.reason);
  let abort: () => void;
  const canceled = new Promise<never>((_resolve, reject) => {
    abort = () => reject(signal.reason);
    signal.addEventListener("abort", abort, { once: true });
  });
  return Promise.race([work, canceled]).finally(() => signal.removeEventListener("abort", abort));
}
