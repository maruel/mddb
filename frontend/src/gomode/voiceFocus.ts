// Applies host focus CAS versions and releases only the lease committed by its bound conversation.

import {
  FrontendVoiceContextConflict,
  type FrontendVoiceContextPublisher,
  type FrontendVoiceContextRelease,
} from "@maruel/gomode/web/FrontendVoiceTools";
import type { UpdateVoiceFocusRequest, UpdateVoiceFocusResponse } from "@sdk/types.gen";

// Bind workspace focus policy to one shared preparation-owned conversation.
export function createVoiceFocusPublisher(
  update: (request: UpdateVoiceFocusRequest, signal: AbortSignal) => Promise<UpdateVoiceFocusResponse>,
  sessionID: string,
  browserSelection: boolean,
  runRelease: FrontendVoiceContextRelease,
): FrontendVoiceContextPublisher {
  let version = 0;
  let published: number | null = null;
  const release = (committed: number) => {
    runRelease((signal) =>
      update({ "openai/session": sessionID, nodeId: "0", version: committed, release: true }, signal),
    );
  };
  return {
    async publish(nodeID, isCurrent, signal) {
      const current = () => isCurrent() && !signal.aborted;
      for (let attempt = 0; attempt < 4 && current(); attempt++) {
        const response = await update(
          { "openai/session": sessionID, nodeId: nodeID ?? "0", version, browserSelection },
          signal,
        );
        if (!current()) {
          // A transport may settle after cancellation. Clear only its committed
          // version; never adopt a stale reply into the current CAS state.
          if (response.applied) release(response.version);
          return;
        }
        version = response.version;
        if (response.applied) {
          published = nodeID === null && !browserSelection ? null : response.version;
          return;
        }
      }
      if (current()) throw new FrontendVoiceContextConflict();
    },
    dispose() {
      if (published !== null) release(published);
      published = null;
    },
  };
}
