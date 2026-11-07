// Publishes validated conversation selections with bounded retries and retired-owner protection.

import type { UpdateVoiceFocusRequest, UpdateVoiceFocusResponse } from "@sdk/types.gen";

export interface VoiceFocusPublisher {
  publish(sessionID: string, nodeID: string | null, isCurrent: () => boolean): Promise<void>;
  dispose(): void;
}

export function createVoiceFocusPublisher(
  update: (request: UpdateVoiceFocusRequest) => Promise<UpdateVoiceFocusResponse>,
  browserSelection: boolean,
): VoiceFocusPublisher {
  let active = true;
  let revision = 0;
  let session = "";
  let version = 0;
  let published: { session: string; version: number } | null = null;
  let tail: Promise<void> = Promise.resolve();
  return {
    publish(sessionID, nodeID, isCurrent) {
      if (!sessionID || sessionID.trim() !== sessionID) return Promise.reject(new Error("Invalid conversation ID."));
      const captured = ++revision;
      const current = () => active && captured === revision && isCurrent();
      const work = async () => {
        if (!current()) return;
        if (session !== sessionID) {
          session = sessionID;
          version = 0;
        }
        for (let attempt = 0; attempt < 4 && current(); attempt++) {
          const response = await update({
            "openai/session": sessionID,
            nodeId: nodeID ?? "0",
            version,
            browserSelection,
          });
          version = response.version;
          if (response.applied)
            published = nodeID === null && !browserSelection ? null : { session: sessionID, version: response.version };
          if (!current()) return;
          if (response.applied) return;
        }
        if (current()) throw new Error("Conversation focus changed concurrently.");
      };
      const result = tail.then(work, work);
      tail = result.catch(() => {});
      return result.then(() => tail);
    },
    dispose() {
      active = false;
      revision++;
      void tail
        .then(async () => {
          // A conflict version may belong to a newer owner. Clear only a lease
          // this publisher successfully wrote, and never retry a retired clear.
          if (published)
            await update({
              "openai/session": published.session,
              nodeId: "0",
              version: published.version,
              release: true,
            });
        })
        .catch((error: unknown) => console.warn("Could not clear retired voice focus", error));
    },
  };
}
