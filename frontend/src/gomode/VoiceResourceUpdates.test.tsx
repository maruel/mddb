// Tests watched-resource replacement, empty-list subscriptions, and late notification isolation.

import { afterEach, it } from "node:test";
import { createSignal } from "solid-js";
import { cleanup, render } from "@solidjs/testing-library";
import { expect, vi } from "@tests/expect";
import { mcpClient, type McpResourceNotification } from "@maruel/gomode/web/McpClient";
import { voiceSession } from "@maruel/gomode/web/VoiceSession";
import VoiceResourceUpdates from "./VoiceResourceUpdates";

afterEach(() => {
  cleanup();
  voiceSession.setState((s) => ({ ...s, connected: false }));
  vi.restoreAllMocks();
});

it("watches the workspace with no document and replaces subscriptions without dispatching retired updates", () => {
  const subscriptions: Array<{ close: ReturnType<typeof vi.fn>; notify: (value: McpResourceNotification) => void }> =
    [];
  const subscribe = vi.spyOn(mcpClient, "subscribeResources").mockImplementation((_options, notify) => {
    const subscription = { close: vi.fn(), notify };
    subscriptions.push(subscription);
    return subscription;
  });
  const inject = vi.spyOn(voiceSession, "injectText").mockImplementation(() => {});
  voiceSession.setState((s) => ({ ...s, connected: true }));
  const [resources, setResources] = createSignal<string[]>([]);
  const view = render(() => (
    <VoiceResourceUpdates runtime={{ mcp: mcpClient, session: voiceSession }} resources={resources} />
  ));
  expect(subscribe).toHaveBeenCalledWith(
    expect.objectContaining({ resourceSubscriptions: [], resourcesListChanged: true }),
    expect.any(Function),
  );
  setResources(["mddb://workspaces/a/nodes/nested"]);
  expect(subscriptions[0]!.close).toHaveBeenCalledOnce();
  subscriptions[0]!.notify({ listChanged: true });
  expect(inject).not.toHaveBeenCalled();
  subscriptions[1]!.notify({ uri: "mddb://workspaces/a/nodes/nested" });
  expect(inject).toHaveBeenCalledOnce();
  view.unmount();
  expect(subscriptions[1]!.close).toHaveBeenCalledOnce();
  subscriptions[1]!.notify({ listChanged: true });
  expect(inject).toHaveBeenCalledOnce();
});
