// Tests current metadata ownership against real providers, root snapshots, local patches, and retired transports.

import { afterEach, beforeEach, it } from "node:test";
import { setImmediate } from "node:timers/promises";
import { createServer, type ServerResponse } from "node:http";
import { cleanup, render, waitFor } from "@solidjs/testing-library";
import { Router, Route } from "@solidjs/router";
import { HostModeProvider } from "@maruel/gomode/web/HostMode";
import type { NodeResponse, UserResponse } from "@sdk/types.gen";
import { WSRoleViewer } from "@sdk/types.gen";
import { expect, vi } from "@tests/expect";
import { AuthProvider, useAuth } from "./AuthContext";
import { WorkspaceDataProvider, useWorkspaceData } from "./WorkspaceDataContext";
import { WorkspaceNavigationProvider } from "./WorkspaceNavigationContext";
import { WorkspaceProvider, useWorkspace } from "./WorkspaceContext";
import { EventSourceProvider } from "./EventSourceContext";
import { I18nProvider } from "../i18n";

const account: UserResponse = {
  id: "account",
  email: "account@example.com",
  name: "Account",
  session_id: "session",
  created: 1,
  modified: 1,
  workspace_id: "workspace",
  workspace_role: WSRoleViewer,
  organization_id: "org",
  organizations: [{ id: "org-member", user_id: "account", organization_id: "org", role: "org:member", created: 1 }],
  workspaces: [
    {
      id: "workspace-member",
      user_id: "account",
      workspace_id: "workspace",
      organization_id: "org",
      role: WSRoleViewer,
      settings: { notifications: true },
      created: 1,
    },
  ],
  settings: { theme: "system", language: "en" },
};
const node = (id: string): NodeResponse => ({
  id,
  title: id,
  created: 1,
  modified: 1,
  has_page: true,
  has_table: false,
});

class Events extends EventTarget {
  static current: Events;
  onopen = null;
  onerror = null;
  constructor(_url: string) {
    super();
    Events.current = this;
  }
  close() {}
  metadata(id: string) {
    this.dispatchEvent(
      new window.MessageEvent("workspace", {
        data: JSON.stringify({
          type: "node_updated",
          node_id: id,
          actor_id: "other",
          modified: 1,
          aspects: ["metadata"],
        }),
      }),
    );
  }
}
let workspace!: ReturnType<typeof useWorkspace>;
let data!: ReturnType<typeof useWorkspaceData>;
let auth!: ReturnType<typeof useAuth>;
let requests: Array<{ id: string; signal: AbortSignal; stopped: boolean }>;
let replies: Map<string, Array<Response | Promise<Response>>>;
let roots: Array<Response | Promise<Response>>;
let childResponses: Array<Response | Promise<Response>>;
const nativeFetch = globalThis.fetch;
let networkOrigin: string | null;
const initial = () => [{ ...node("one"), icon: "old", has_children: true }, node("two")];
function Probe() {
  workspace = useWorkspace();
  data = useWorkspaceData();
  auth = useAuth();
  return null;
}
async function mount() {
  window.history.replaceState(null, "", "/w/workspace");
  render(() => (
    <I18nProvider initialLocale="en">
      <AuthProvider>
        <Router
          root={(props) => (
            <HostModeProvider>
              <WorkspaceDataProvider>
                <WorkspaceNavigationProvider>{props.children}</WorkspaceNavigationProvider>
              </WorkspaceDataProvider>
            </HostModeProvider>
          )}
        >
          <Route
            path="/w/:wsId/*"
            component={() => (
              <EventSourceProvider>
                <WorkspaceProvider>
                  <Probe />
                </WorkspaceProvider>
              </EventSourceProvider>
            )}
          />
        </Router>
      </AuthProvider>
    </I18nProvider>
  ));
  await waitFor(() => expect(data.ready()).toBe(true));
}
function body(id: string) {
  const pending = Promise.withResolvers<NodeResponse>();
  const started = Promise.withResolvers<void>();
  const response = Response.json({});
  vi.spyOn(response, "json").mockImplementation(() => {
    started.resolve();
    return pending.promise;
  });
  replies.set(id, [...(replies.get(id) ?? []), response]);
  return { ...pending, started: started.promise };
}
function transport(id: string) {
  const pending = Promise.withResolvers<Response>();
  replies.set(id, [...(replies.get(id) ?? []), pending.promise]);
  return pending;
}
async function metadata(id: string) {
  const count = requests.length;
  Events.current.metadata(id);
  await waitFor(() => expect(requests.length).toBe(count + 1));
  return requests.at(-1)!;
}
beforeEach(() => {
  networkOrigin = null;
  requests = [];
  replies = new Map();
  roots = [Response.json({ nodes: initial() })];
  childResponses = [];
  vi.stubGlobal("EventSource", Events);
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.endsWith("/auth/me")) return Response.json(account);
    if (url.endsWith("/nodes/0/children")) {
      const response = roots.shift();
      if (!response) throw new Error("Unexpected root request");
      return response;
    }
    if (url.endsWith("/children"))
      return childResponses.shift() ?? Response.json({ nodes: [{ ...node("child"), parent_id: "one" }] });
    if (init?.method === "POST") return Response.json({});
    const id = url.split("/").at(-1)!;
    const signal = init?.signal;
    if (!signal) throw new Error("Missing SDK signal");
    const request = { id, signal, stopped: false };
    requests.push(request);
    if (networkOrigin && id === "one") return nativeFetch(networkOrigin, init);
    const reply = replies.get(id)?.shift();
    if (!reply) return Response.json({ ...node(id), ...(id === "child" ? { parent_id: "one" } : {}) });
    return new Promise<Response>((resolve, reject) => {
      signal.addEventListener(
        "abort",
        () => {
          request.stopped = true;
          reject(signal.reason);
        },
        { once: true },
      );
      Promise.resolve(reply).then(resolve, reject);
    });
  });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("retires same-node metadata before its transport gate opens while unrelated nodes remain live", async () => {
  await mount();
  const first = transport("one");
  const firstRequest = await metadata("one");
  const other = transport("two");
  const otherRequest = await metadata("two");
  const newest = body("one");
  await metadata("one");
  await newest.started;
  expect(firstRequest.signal.aborted).toBe(true);
  expect(firstRequest.stopped).toBe(true);
  expect(otherRequest.signal.aborted).toBe(false);
  newest.resolve({ ...node("one"), title: "newest" });
  await waitFor(() => expect(workspace.nodes[0]?.title).toBe("newest"));
  first.resolve(Response.json({ ...node("one"), title: "obsolete" }));
  other.resolve(Response.json({ ...node("two"), title: "independent" }));
  await waitFor(() => expect(workspace.nodes[1]?.title).toBe("independent"));
  expect(workspace.nodes[0]?.title).toBe("newest");
});

it("ignores superseded JSON completion and clears metadata while preserving loaded descendants, breadcrumbs and document", async () => {
  await mount();
  await workspace.fetchNodeChildren("one");
  await workspace.loadNode("child", { refresh: false });
  const parent = workspace.nodes[0]!;
  const children = parent.children;
  const child = children![0];
  const selected = workspace.selectedNodeData();
  const old = body("one");
  const oldRequest = await metadata("one");
  await old.started;
  const latest = body("one");
  await metadata("one");
  await latest.started;
  latest.resolve({ ...node("one"), title: "fresh", has_page: false });
  await waitFor(() => expect(parent.title).toBe("fresh"));
  old.resolve({ ...node("one"), title: "obsolete", icon: "obsolete" });
  await setImmediate();
  expect(oldRequest.signal.aborted).toBe(true);
  expect(workspace.nodes[0]).toBe(parent);
  expect(parent.children).toBe(children);
  expect(children![0]).toBe(child);
  expect(parent.icon).toBeUndefined();
  expect(parent.has_page).toBe(false);
  expect(workspace.breadcrumbPath().map((n) => n.title)).toEqual(["fresh", "child"]);
  expect(workspace.selectedNodeData()).toBe(selected);
});

it("retires metadata at root-read start and queues new metadata behind the older root snapshot", async () => {
  await mount();
  const old = body("one");
  const oldRequest = await metadata("one");
  await old.started;
  const root = Promise.withResolvers<Response>();
  roots.push(root.promise);
  const refresh = data.refreshRoots();
  expect(oldRequest.signal.aborted).toBe(true);
  const queued = body("one");
  const count = requests.length;
  Events.current.metadata("one");
  await setImmediate();
  expect(requests).toHaveLength(count);
  root.resolve(Response.json({ nodes: [{ ...node("one"), title: "root snapshot" }, node("two")] }));
  await refresh;
  await queued.started;
  queued.resolve({ ...node("one"), title: "after root" });
  await waitFor(() => expect(workspace.nodes[0]?.title).toBe("after root"));
  old.resolve({ ...node("one"), title: "before root" });
  await setImmediate();
  expect(workspace.nodes[0]?.title).toBe("after root");
});

it("does not request queued metadata for a node removed by the pending root snapshot", async () => {
  await mount();
  const root = Promise.withResolvers<Response>();
  roots.push(root.promise);
  const refresh = data.refreshRoots();
  Events.current.metadata("one");
  await setImmediate();
  root.resolve(Response.json({ nodes: [node("two")] }));
  await refresh;
  await setImmediate();
  expect(requests).toEqual([]);
  expect(workspace.nodes.map((n) => n.id)).toEqual(["two"]);
});

it("retires local patches and deletion subtrees without canceling independent metadata", async () => {
  await mount();
  await workspace.fetchNodeChildren("one");
  await workspace.loadNode("child", { refresh: false });
  const title = body("one");
  const titleRequest = await metadata("one");
  await title.started;
  workspace.updateNodeTitle("one", "local");
  expect(titleRequest.signal.aborted).toBe(true);
  title.resolve({ ...node("one"), title: "old" });
  await setImmediate();
  expect(workspace.nodes[0]?.title).toBe("local");
  const icon = body("one");
  const iconRequest = await metadata("one");
  await icon.started;
  workspace.updateNodeIcon("one", "local-icon");
  expect(iconRequest.signal.aborted).toBe(true);
  icon.resolve({ ...node("one"), icon: "old-icon" });
  await setImmediate();
  expect(workspace.nodes[0]?.icon).toBe("local-icon");
  const parent = transport("one");
  const parentRequest = await metadata("one");
  const child = body("child");
  const childRequest = await metadata("child");
  await child.started;
  const other = transport("two");
  const otherRequest = await metadata("two");
  const crumbs = workspace.breadcrumbPath();
  workspace.removeNode("one");
  expect(parentRequest.signal.aborted).toBe(true);
  expect(parentRequest.stopped).toBe(true);
  expect(childRequest.signal.aborted).toBe(true);
  expect(otherRequest.signal.aborted).toBe(false);
  parent.resolve(Response.json(node("one")));
  child.resolve({ ...node("child"), title: "removed" });
  other.resolve(Response.json({ ...node("two"), title: "live" }));
  await waitFor(() => expect(workspace.nodes[0]?.title).toBe("live"));
  expect(workspace.breadcrumbPath()).toEqual(crumbs);
});

it("keeps stale errors silent, exposes current errors, and ignores missing-node responses", async () => {
  await mount();
  const old = body("one");
  await metadata("one");
  await old.started;
  const current = body("one");
  await metadata("one");
  await current.started;
  current.reject(new Error("current failure"));
  await waitFor(() => expect(workspace.loadError()).toContain("current failure"));
  old.reject(new Error("obsolete failure"));
  await setImmediate();
  expect(workspace.loadError()).toContain("current failure");
  workspace.clearErrors();
  replies.set("one", [Response.json({ error: "missing" }, { status: 404 })]);
  await metadata("one");
  await setImmediate();
  expect(workspace.loadError()).toBeNull();
});

it("retires requests on workspace replacement and disposal before held transport gates release", async () => {
  await mount();
  const old = body("one");
  const request = await metadata("one");
  await old.started;
  roots.push(Response.json({ nodes: initial() }));
  auth.setUser({ ...account, workspace_id: "replacement" });
  await waitFor(() => expect(request.signal.aborted).toBe(true));
  await waitFor(() => expect(data.ready()).toBe(true));
  old.reject(new Error("retired workspace"));
  await setImmediate();
  expect(workspace.loadError()).toBeNull();
  const held = transport("one");
  const heldRequest = await metadata("one");
  cleanup();
  expect(heldRequest.signal.aborted).toBe(true);
  expect(heldRequest.stopped).toBe(true);
  held.resolve(Response.json(node("one")));
  await setImmediate();
});

it("retires a pending metadata body before applying a successful local move", async () => {
  await mount();
  const pending = body("one");
  const request = await metadata("one");
  await pending.started;
  await workspace.moveNode("one", "two");
  expect(request.signal.aborted).toBe(true);
  pending.resolve({ ...node("one"), parent_id: "0", title: "stale" });
  await setImmediate();
  expect(workspace.nodes.map((n) => n.id)).toEqual(["two"]);
  // An unloaded destination stays unloaded; a stale response cannot resurrect
  // the removed root or rewrite retained navigation metadata.
  expect(workspace.nodes[0]?.children).toBeUndefined();
});

it("keeps a replacement owner live when an obsolete body settles first and preserves unrelated errors", async () => {
  await mount();
  const old = body("one");
  await metadata("one");
  await old.started;
  const latest = body("one");
  const latestRequest = await metadata("one");
  await latest.started;
  old.reject(new Error("obsolete"));
  await setImmediate();
  expect(workspace.loadError()).toBeNull();
  expect(latestRequest.signal.aborted).toBe(false);
  const failed = body("two");
  await metadata("two");
  await failed.started;
  failed.reject(new Error("unrelated"));
  await waitFor(() => expect(workspace.loadError()).toContain("unrelated"));
  latest.resolve({ ...node("one"), title: "latest" });
  await waitFor(() => expect(workspace.nodes[0]?.title).toBe("latest"));
  expect(workspace.loadError()).toContain("unrelated");
});

it("leaves canonical preparation failure with its owner when a queued metadata read cannot start", async () => {
  await mount();
  const pending = Promise.withResolvers<Response>();
  roots.push(pending.promise);
  const refresh = data.refreshRoots();
  const observed = refresh.catch(() => undefined);
  Events.current.metadata("one");
  await setImmediate();
  pending.resolve(Response.json({ error: "root rejected" }, { status: 403 }));
  await observed;
  await setImmediate();
  expect(data.preparationError()).not.toBeNull();
  expect(workspace.loadError()).toBeNull();
  expect(requests).toEqual([]);
});

it("aborts the underlying HTTP request before the server releases its gated response", async () => {
  await mount();
  let response: ServerResponse | null = null;
  let stopped = false;
  const server = createServer((_request, res) => {
    response = res;
    res.on("close", () => {
      stopped = true;
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  try {
    const address = server.address();
    if (!address || typeof address === "string") throw new Error("Missing HTTP listen address");
    networkOrigin = `http://127.0.0.1:${address.port}/metadata`;
    await metadata("one");
    await waitFor(() => expect(response).not.toBeNull());
    networkOrigin = null;
    const latest = body("one");
    await metadata("one");
    await latest.started;
    // The server is still holding the response. Cleanup must not cause this.
    await waitFor(() => expect(stopped).toBe(true));
    latest.resolve({ ...node("one"), title: "current" });
    await waitFor(() => expect(workspace.nodes[0]?.title).toBe("current"));
  } finally {
    server.closeAllConnections();
    await new Promise<void>((resolve, reject) => server.close((error) => (error ? reject(error) : resolve())));
  }
});

it("retires moved-node parents before child-presence patches while unrelated metadata stays live", async () => {
  roots = [Response.json({ nodes: [...initial(), node("other")] })];
  await mount();
  await workspace.fetchNodeChildren("one");
  const source = body("one");
  const sourceRequest = await metadata("one");
  await source.started;
  const destination = body("two");
  const destinationRequest = await metadata("two");
  await destination.started;
  const unrelated = body("other");
  const unrelatedRequest = await metadata("other");
  await unrelated.started;
  await workspace.moveNode("child", "two");
  expect(sourceRequest.signal.aborted).toBe(true);
  expect(destinationRequest.signal.aborted).toBe(true);
  expect(unrelatedRequest.signal.aborted).toBe(false);
  source.resolve({ ...node("one"), has_children: true });
  destination.resolve(node("two"));
  unrelated.resolve({ ...node("other"), title: "unrelated refresh" });
  await waitFor(() => expect(workspace.nodes[2]?.title).toBe("unrelated refresh"));
  expect(workspace.nodes[0]?.children).toEqual([]);
  expect(workspace.nodes[0]?.has_children).toBe(false);
  expect(workspace.nodes[1]?.children).toBeUndefined();
  expect(workspace.nodes[1]?.has_children).toBe(true);
});

it("retires the parent metadata before deleting its last loaded child", async () => {
  await mount();
  await workspace.fetchNodeChildren("one");
  const parent = body("one");
  const parentRequest = await metadata("one");
  await parent.started;
  const unrelated = body("two");
  const unrelatedRequest = await metadata("two");
  await unrelated.started;
  workspace.removeNode("child");
  expect(parentRequest.signal.aborted).toBe(true);
  expect(unrelatedRequest.signal.aborted).toBe(false);
  parent.resolve({ ...node("one"), has_children: true });
  unrelated.resolve({ ...node("two"), title: "unrelated" });
  await waitFor(() => expect(workspace.nodes[1]?.title).toBe("unrelated"));
  expect(workspace.nodes[0]?.children).toEqual([]);
  expect(workspace.nodes[0]?.has_children).toBe(false);
});

it("retires parent and replaced descendant metadata before publishing a child snapshot", async () => {
  await mount();
  await workspace.fetchNodeChildren("one");
  const parent = body("one");
  const parentRequest = await metadata("one");
  await parent.started;
  const child = body("child");
  const childRequest = await metadata("child");
  await child.started;
  const unrelated = body("two");
  const unrelatedRequest = await metadata("two");
  await unrelated.started;
  childResponses.push(Response.json({ nodes: [{ ...node("child"), parent_id: "one", title: "fresh child" }] }));
  await workspace.fetchNodeChildren("one");
  expect(parentRequest.signal.aborted).toBe(true);
  expect(childRequest.signal.aborted).toBe(true);
  expect(unrelatedRequest.signal.aborted).toBe(false);
  parent.resolve({ ...node("one"), has_children: false });
  child.resolve({ ...node("child"), title: "stale child" });
  unrelated.resolve({ ...node("two"), title: "unrelated" });
  await waitFor(() => expect(workspace.nodes[1]?.title).toBe("unrelated"));
  expect(workspace.nodes[0]?.has_children).toBe(true);
  expect(workspace.nodes[0]?.children?.[0]?.title).toBe("fresh child");
});

async function loadDeepTree() {
  roots = [Response.json({ nodes: [...initial(), node("other")] })];
  await mount();
  await workspace.fetchNodeChildren("one");
  childResponses.push(Response.json({ nodes: [{ ...node("grandchild"), parent_id: "child" }] }));
  await workspace.fetchNodeChildren("child");
}
async function reexpandMovedTree() {
  childResponses.push(Response.json({ nodes: [{ ...node("one"), parent_id: "two", has_children: true }] }));
  await workspace.fetchNodeChildren("two");
  childResponses.push(Response.json({ nodes: [{ ...node("child"), parent_id: "one", has_children: true }] }));
  await workspace.fetchNodeChildren("one");
  childResponses.push(
    Response.json({ nodes: [{ ...node("grandchild"), parent_id: "child", title: "fresh after reentry" }] }),
  );
  await workspace.fetchNodeChildren("child");
}
for (const outcome of ["error", "success"] as const) {
  it(`a detached descendant cannot regain stale metadata ${outcome} ownership after lazy reentry`, async () => {
    await loadDeepTree();
    const pending = body("grandchild");
    const descendantRequest = await metadata("grandchild");
    await pending.started;
    const unrelated = body("other");
    const unrelatedRequest = await metadata("other");
    await unrelated.started;
    await workspace.moveNode("one", "two");
    expect(workspace.nodes.map((n) => n.id)).toEqual(["two", "other"]);
    await reexpandMovedTree();
    if (outcome === "error") pending.reject(new Error("obsolete descendant failure"));
    else pending.resolve({ ...node("grandchild"), parent_id: "child", title: "obsolete descendant" });
    unrelated.resolve({ ...node("other"), title: "unrelated remains live" });
    await waitFor(() => expect(workspace.nodes[1]?.title).toBe("unrelated remains live"));
    expect(workspace.nodes[0]?.children?.[0]?.children?.[0]?.children?.[0]?.title).toBe("fresh after reentry");
    expect(workspace.loadError()).toBeNull();
    expect(descendantRequest.signal.aborted).toBe(true);
    expect(unrelatedRequest.signal.aborted).toBe(false);
  });
}

it("stops detached descendant transport before its response is released without canceling unrelated requests", async () => {
  await loadDeepTree();
  const pending = transport("grandchild");
  const descendantRequest = await metadata("grandchild");
  const unrelated = transport("other");
  const unrelatedRequest = await metadata("other");
  await workspace.moveNode("one", "two");
  expect(descendantRequest.signal.aborted).toBe(true);
  expect(descendantRequest.stopped).toBe(true);
  expect(unrelatedRequest.signal.aborted).toBe(false);
  pending.resolve(Response.json({ ...node("grandchild"), title: "obsolete" }));
  unrelated.resolve(Response.json({ ...node("other"), title: "unrelated remains live" }));
  await waitFor(() => expect(workspace.nodes[1]?.title).toBe("unrelated remains live"));
});
