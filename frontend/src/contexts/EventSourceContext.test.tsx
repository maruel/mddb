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
  user: () => testUser,
  token: () => "test-token",
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

afterEach(() => {
  cleanup();
  MockEventSource.instances = [];
  vi.unstubAllGlobals();
});

describe("EventSourceProvider", () => {
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
        </EventSourceProvider>
      </AuthContext.Provider>
    ));

    expect(MockEventSource.instances).toHaveLength(1);
    const eventSource = MockEventSource.instances[0];
    if (!eventSource) {
      throw new Error("EventSourceProvider did not create an event source");
    }
    expect(eventSource.url).toBe("/api/v1/workspaces/workspace-1/events?token=test-token");
    expect(screen.getByTestId("connection-status")).toHaveTextContent("Reconnecting");

    eventSource.open();
    expect(screen.getByTestId("connection-status")).toHaveTextContent("Connected");

    eventSource.fail();
    expect(screen.getByTestId("connection-status")).toHaveTextContent("Reconnecting");

    eventSource.open();
    expect(screen.getByTestId("connection-status")).toHaveTextContent("Connected");
  });
});
