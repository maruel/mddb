// Tests workspace event delivery and SSE connection recovery.

import { afterEach, describe, it } from "node:test";
import { expect, vi } from "@tests/expect";
import { cleanup, render, screen } from "@solidjs/testing-library";
import { AuthContext, type AuthContextValue } from "./AuthContext";
import { EventSourceProvider, useEventSource } from "./EventSourceContext";
import type { UserResponse } from "@sdk/types.gen";
import { localWriteHeaders } from "../useApi";

const testUser: UserResponse = {
  id: "user-1",
  email: "test@example.com",
  name: "Test User",
  organization_id: "org-1",
  org_role: "org:member",
  workspace_id: "workspace-1",
  workspace_name: "Test Workspace",
  workspace_role: "ws:viewer",
  organizations: [],
  workspaces: [],
} as unknown as UserResponse;

// Canned auth state provided through the real AuthContext instead of a module mock.
const authValue: AuthContextValue = {
  frontendIdentity: () => null,
  user: () => testUser,
  identity: () => 1,
  ready: () => true,
  api: () => {
    throw new Error("api is not used by these tests");
  },
  wsApi: () => null,
  orgApi: () => null,
  login: () => undefined,
  logout: async () => undefined,
  setUser: () => undefined,
  refreshUser: async () => undefined,
  reloadIdentity: () => undefined,
  refreshIdentity: async () => false,
};

class MockEventSource {
  static instances: MockEventSource[] = [];
  onopen: ((event: Event) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  readonly close = vi.fn();
  readonly listeners = new Map<string, (event: MessageEvent<string>) => void>();

  constructor(readonly url: string) {
    MockEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: (event: MessageEvent<string>) => void) {
    this.listeners.set(type, listener);
  }

  workspace(actorId: string, source: string | null) {
    this.listeners.get("workspace")?.(
      new MessageEvent("workspace", {
        data: JSON.stringify({
          type: "node_updated",
          node_id: "node-1",
          actor_id: actorId,
          modified: 123,
          request_id: source ?? undefined,
        }),
      }),
    );
  }

  open() {
    this.onopen?.(new Event("open"));
  }

  fail() {
    this.onerror?.(new Event("error"));
  }
}

function ConnectionStatus() {
  const { connected } = useEventSource();
  return <output data-testid="connection-status">{connected() ? "Connected" : "Reconnecting"}</output>;
}

function LatestEvent() {
  const { lastEvent } = useEventSource();
  return <output data-testid="latest-event">{lastEvent()?.actor_id}</output>;
}

function LatestEventType() {
  const { lastEvent } = useEventSource();
  return <output data-testid="latest-type">{lastEvent()?.type}</output>;
}

afterEach(() => {
  vi.useRealTimers();
  cleanup();
  MockEventSource.instances = [];
  vi.unstubAllGlobals();
});

describe("EventSourceProvider", () => {
  it("waits offline, replaces routes immediately, and ignores retired callbacks", async (t) => {
    vi.useFakeTimers();
    let online = false;
    t.mock.getter(navigator, "onLine", () => online);
    const network = Object.assign(new EventTarget(), { type: "wifi" });
    Object.defineProperty(navigator, "connection", { configurable: true, value: network });
    t.after(() => Reflect.deleteProperty(navigator, "connection"));
    vi.stubGlobal("EventSource", MockEventSource);
    const view = render(() => (
      <AuthContext.Provider value={authValue}>
        <EventSourceProvider>
          <ConnectionStatus />
          <LatestEvent />
          <LatestEventType />
        </EventSourceProvider>
      </AuthContext.Provider>
    ));
    expect(MockEventSource.instances).toHaveLength(0);
    online = true;
    window.dispatchEvent(new Event("online"));
    const first = MockEventSource.instances[0]!;
    first.open();
    network.type = "cellular";
    network.dispatchEvent(new Event("change"));
    expect(first.close).toHaveBeenCalledOnce();
    const next = MockEventSource.instances[1]!;
    next.open();
    expect(screen.getByTestId("latest-type")).toHaveTextContent("resync");
    first.fail();
    first.workspace("retired", null);
    expect(screen.getByTestId("connection-status")).toHaveTextContent("Connected");
    expect(screen.getByTestId("latest-event")).toHaveTextContent("0");
    network.dispatchEvent(new Event("change"));
    expect(MockEventSource.instances).toHaveLength(2);
    online = false;
    window.dispatchEvent(new Event("offline"));
    await vi.advanceTimersByTimeAsync(300_000);
    expect(next.close).toHaveBeenCalledOnce();
    expect(MockEventSource.instances).toHaveLength(2);
    view.unmount();
    online = true;
    window.dispatchEvent(new Event("online"));
    expect(MockEventSource.instances).toHaveLength(2);
  });

  it("replaces a silent stream but keeps heartbeat traffic alive", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("EventSource", MockEventSource);
    render(() => (
      <AuthContext.Provider value={authValue}>
        <EventSourceProvider>
          <ConnectionStatus />
          <LatestEventType />
        </EventSourceProvider>
      </AuthContext.Provider>
    ));
    const first = MockEventSource.instances[0]!;
    first.open();
    for (let i = 0; i < 4; i++) {
      await vi.advanceTimersByTimeAsync(30_000);
      first.listeners.get("heartbeat")?.(new MessageEvent("heartbeat", { data: "{}" }));
    }
    expect(MockEventSource.instances).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(90_000);
    expect(first.close).toHaveBeenCalledOnce();
    expect(MockEventSource.instances).toHaveLength(2);
    MockEventSource.instances[1]!.open();
    expect(screen.getByTestId("latest-type")).toHaveTextContent("resync");
  });

  it("leaves reported connection errors to native retry and terminal HTTP handling", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("EventSource", MockEventSource);
    render(() => (
      <AuthContext.Provider value={authValue}>
        <EventSourceProvider>
          <ConnectionStatus />
        </EventSourceProvider>
      </AuthContext.Provider>
    ));
    const first = MockEventSource.instances[0]!;
    first.open();
    first.fail();
    await vi.advanceTimersByTimeAsync(180_000);
    expect(MockEventSource.instances).toHaveLength(1);
    expect(first.close).not.toHaveBeenCalled();
    first.open();
    expect(screen.getByTestId("connection-status")).toHaveTextContent("Connected");
  });

  for (const recovery of ["stall", "offline", "native"] as const) {
    it(`resynchronizes after ${recovery} before the initial stream opens`, async (t) => {
      vi.useFakeTimers();
      let online = true;
      t.mock.getter(navigator, "onLine", () => online);
      vi.stubGlobal("EventSource", MockEventSource);
      render(() => (
        <AuthContext.Provider value={authValue}>
          <EventSourceProvider>
            <LatestEventType />
          </EventSourceProvider>
        </AuthContext.Provider>
      ));
      if (recovery === "stall") await vi.advanceTimersByTimeAsync(90_000);
      else if (recovery === "offline") {
        online = false;
        window.dispatchEvent(new Event("offline"));
        online = true;
        window.dispatchEvent(new Event("online"));
      } else MockEventSource.instances[0]!.fail();
      MockEventSource.instances.at(-1)!.open();
      expect(screen.getByTestId("latest-type")).toHaveTextContent("resync");
    });
  }

  it("delivers edits from the same account as well as other users", () => {
    vi.stubGlobal("EventSource", MockEventSource);
    render(() => (
      <AuthContext.Provider value={authValue}>
        <EventSourceProvider>
          <LatestEvent />
        </EventSourceProvider>
      </AuthContext.Provider>
    ));
    const source = MockEventSource.instances[0]!;
    source.workspace("user-2", null);
    expect(screen.getByTestId("latest-event")).toHaveTextContent("user-2");
    source.workspace(testUser.id, null);
    expect(screen.getByTestId("latest-event")).toHaveTextContent(testUser.id);
    const requestID = localWriteHeaders(undefined).get("X-Request-ID");
    expect(requestID).toBeTruthy();
    source.workspace("user-2", requestID);
    expect(screen.getByTestId("latest-event")).toHaveTextContent("user-2");
    source.workspace(testUser.id, requestID);
    expect(screen.getByTestId("latest-event")).toHaveTextContent("user-2");
    source.workspace(testUser.id, "independent-mcp-request");
    expect(screen.getByTestId("latest-event")).toHaveTextContent(testUser.id);
  });

  it("reports reconnecting after a connection failure and connected after recovery", () => {
    vi.stubGlobal("EventSource", MockEventSource);

    render(() => (
      <AuthContext.Provider value={authValue}>
        <EventSourceProvider>
          <ConnectionStatus />
          <LatestEventType />
        </EventSourceProvider>
      </AuthContext.Provider>
    ));

    expect(MockEventSource.instances).toHaveLength(1);
    const eventSource = MockEventSource.instances[0];
    if (!eventSource) {
      throw new Error("EventSourceProvider did not create an event source");
    }
    expect(eventSource.url).toBe("/api/v1/workspaces/workspace-1/events");
    expect(screen.getByTestId("connection-status")).toHaveTextContent("Reconnecting");

    eventSource.open();
    expect(screen.getByTestId("connection-status")).toHaveTextContent("Connected");
    expect(screen.getByTestId("latest-type")).toBeEmptyDOMElement();

    eventSource.fail();
    expect(screen.getByTestId("connection-status")).toHaveTextContent("Reconnecting");

    eventSource.open();
    expect(screen.getByTestId("connection-status")).toHaveTextContent("Connected");
    expect(screen.getByTestId("latest-type")).toHaveTextContent("resync");
  });
});
