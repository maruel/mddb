// Tests selected-node and metadata ownership, root snapshots, local patches, and retired transports.

import { afterEach, beforeEach, it } from "node:test";
import { setImmediate } from "node:timers/promises";
import { createServer, type ServerResponse } from "node:http";
import { cleanup, render, waitFor } from "@solidjs/testing-library";
import { Router, Route, useLocation, useNavigate } from "@solidjs/router";
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
  moved(id: string, parentId: string) {
    this.dispatchEvent(
      new window.MessageEvent("workspace", {
        data: JSON.stringify({ type: "node_moved", node_id: id, parent_id: parentId, actor_id: "other", modified: 2 }),
      }),
    );
  }
}
let workspace!: ReturnType<typeof useWorkspace>;
let data!: ReturnType<typeof useWorkspaceData>;
let auth!: ReturnType<typeof useAuth>;
let navigate!: ReturnType<typeof useNavigate>;
let currentPath!: () => string;
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
  navigate = useNavigate();
  const location = useLocation();
  currentPath = () => location.pathname;
  return null;
}
async function mount(ready: boolean) {
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
  await waitFor(() => expect(auth.user()?.workspace_id).toBe("workspace"));
  if (ready) await waitFor(() => expect(data.ready()).toBe(true));
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

it("keeps the deep-linked document independent of delayed preparation failure and retry", async () => {
  const root = Promise.withResolvers<Response>();
  roots = [root.promise];
  await mount(false);
  expect(data.loadingNodes()).toBe(true);
  const loading = workspace.loadNode("child", { refresh: false });
  await waitFor(() => expect(workspace.selectedNodeId()).toBe("child"));
  root.resolve(Response.json({ error: { message: "Roots unavailable" } }, { status: 404 }));
  await loading;
  expect(workspace.selectedNodeId()).toBe("child");
  expect(workspace.failedNodeId()).toBeNull();
  expect(data.preparationError()).not.toBeNull();
  roots.push(Response.json({ nodes: initial() }));
  await data.retryPreparation();
  await waitFor(() => expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["one", "child"]));
  expect(workspace.selectedNodeId()).toBe("child");
  expect(requests.filter((item) => item.id === "child")).toHaveLength(1);
  expect(workspace.loadError()).toBeNull();
});

it("keeps navigation cancellation independent of failing roots", async () => {
  const root = Promise.withResolvers<Response>();
  roots = [root.promise];
  await mount(false);
  const held = transport("one");
  const obsolete = workspace.loadNode("one", { refresh: false });
  await waitFor(() => expect(requests).toHaveLength(1));
  const request = requests[0]!;
  const selected = workspace.loadNode("two", { refresh: false });
  await waitFor(() => expect(workspace.selectedNodeId()).toBe("two"));
  expect(request.signal.aborted).toBe(true);
  root.resolve(Response.json({ error: { message: "Roots unavailable" } }, { status: 404 }));
  await selected;
  held.resolve(Response.json(node("one")));
  await obsolete;
  roots.push(Response.json({ nodes: initial() }));
  await data.retryPreparation();
  expect(workspace.selectedNodeId()).toBe("two");
  expect(workspace.failedNodeId()).toBeNull();
  expect(workspace.loadError()).toBeNull();
});

it("restarts an unaccepted document on root retry without admitting its retired body", async () => {
  const root = Promise.withResolvers<Response>();
  roots = [root.promise];
  await mount(false);
  const old = body("child");
  const obsolete = workspace.loadNode("child", { refresh: false });
  await old.started;
  const request = requests.at(-1)!;
  root.resolve(Response.json({ error: { message: "Roots unavailable" } }, { status: 404 }));
  await waitFor(() => expect(data.preparationError()).not.toBeNull());
  roots.push(Response.json({ nodes: initial() }));
  replies.set("child", [Response.json({ ...node("child"), parent_id: "one", title: "Current document" })]);
  await data.retryPreparation();
  await waitFor(() => expect(workspace.selectedNodeData()?.title).toBe("Current document"));
  expect(request.signal.aborted).toBe(true);
  old.resolve({ ...node("child"), title: "Retired document" });
  await obsolete;
  expect(workspace.selectedNodeData()?.title).toBe("Current document");
});

it("retires selected-node transport before the superseded response is released", async () => {
  await mount(true);
  const held = transport("one");
  const obsolete = workspace.loadNode("one", { refresh: false });
  await waitFor(() => expect(requests).toHaveLength(1));
  const request = requests[0]!;
  const selected = await workspace.loadNode("two", { refresh: false });
  expect(request.signal.aborted).toBe(true);
  expect(request.stopped).toBe(true);
  expect(selected?.id).toBe("two");
  held.resolve(Response.json(node("one")));
  expect(await obsolete).toBeUndefined();
  expect(workspace.selectedNodeId()).toBe("two");
  expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["two"]);
});

it("joins an active selection and reuses the selected snapshot until an explicit refresh", async () => {
  await mount(true);
  const held = transport("one");
  const first = workspace.loadNode("one", { refresh: false });
  await waitFor(() => expect(requests).toHaveLength(1));
  const joined = workspace.loadNode("one", { refresh: false });
  held.resolve(Response.json(node("one")));
  expect((await first)?.id).toBe("one");
  expect((await joined)?.id).toBe("one");
  expect((await workspace.loadNode("one", { refresh: false }))?.id).toBe("one");
  expect(requests).toHaveLength(1);
  replies.set("one", [Response.json({ ...node("one"), title: "refreshed" })]);
  await workspace.loadNode("one", { refresh: true });
  expect(requests).toHaveLength(2);
  expect(workspace.selectedNodeData()?.title).toBe("refreshed");
});

it("retires breadcrumb transport on route departure without publishing stale paths", async () => {
  await mount(true);
  const held = transport("one");
  const loading = workspace.loadNode("child", { refresh: false });
  await waitFor(() => expect(requests).toHaveLength(2));
  const parent = requests[1]!;
  navigate("/w/workspace/two");
  await waitFor(() => expect(parent.signal.aborted).toBe(true));
  expect(parent.stopped).toBe(true);
  await workspace.loadNode("two", { refresh: false });
  held.resolve(Response.json(node("one")));
  await loading;
  expect(workspace.selectedNodeId()).toBe("two");
  expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["two"]);
});

it("keeps partial breadcrumbs when the current ancestor read fails", async () => {
  await mount(true);
  replies.set("one", [Response.json({ error: "ancestor unavailable" }, { status: 404 })]);
  expect((await workspace.loadNode("child", { refresh: false }))?.id).toBe("child");
  expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["child"]);
  expect(workspace.failedNodeId()).toBeNull();
  expect(workspace.loadError()).toBeNull();
  const count = requests.length;
  await workspace.loadNode("child", { refresh: false });
  expect(requests).toHaveLength(count);
});

it("preserves valid ancestor membership until a same-node refresh completes its path", async () => {
  await mount(true);
  await workspace.loadNode("child", { refresh: false });
  const parent = body("one");
  replies.set("child", [Response.json({ ...node("child"), parent_id: "one", title: "refreshed" })]);
  const refreshing = workspace.loadNode("child", { refresh: true });
  await parent.started;
  expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["one", "child"]);
  expect(workspace.breadcrumbPath().at(-1)?.title).toBe("refreshed");
  const latest = { ...node("child"), parent_id: "one", title: "optimistic during refresh" };
  workspace.updateSelectedNode(latest);
  parent.resolve({ ...node("one"), title: "updated parent" });
  await refreshing;
  expect(workspace.selectedNodeData()).toBe(latest);
  expect(workspace.breadcrumbPath().map((item) => item.title)).toEqual(["updated parent", "optimistic during refresh"]);
});

it("retires captured ancestry when the selected node is moved", async () => {
  await mount(true);
  await workspace.fetchNodeChildren("one");
  const old = body("one");
  const loading = workspace.loadNode("child", { refresh: false });
  await old.started;
  const request = requests.at(-1)!;
  await workspace.moveNode("child", "two");
  expect(request.signal.aborted).toBe(true);
  old.resolve(node("one"));
  await loading;
  await waitFor(() => expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["two", "child"]));
  expect(workspace.selectedNodeData()?.parent_id).toBe("two");
});

it("retires captured ancestry when a loaded ancestor is moved", async () => {
  await loadDeepTree();
  replies.set("grandchild", [Response.json({ ...node("grandchild"), parent_id: "child" })]);
  const old = body("child");
  const loading = workspace.loadNode("grandchild", { refresh: false });
  await old.started;
  const request = requests.at(-1)!;
  const selected = workspace.selectedNodeData();
  replies.set("one", [Response.json({ ...node("one"), parent_id: "two" })]);
  await workspace.moveNode("one", "two");
  expect(request.signal.aborted).toBe(true);
  old.resolve({ ...node("child"), parent_id: "one" });
  await loading;
  await waitFor(() =>
    expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["two", "one", "child", "grandchild"]),
  );
  expect(workspace.selectedNodeData()).toBe(selected);
});

it("keeps acknowledged title and icon changes without retiring unchanged ancestry", async () => {
  await mount(true);
  const old = body("one");
  const loading = workspace.loadNode("child", { refresh: false });
  await old.started;
  const request = requests.at(-1)!;
  workspace.updateNodeTitle("one", "acknowledged title");
  workspace.updateNodeIcon("one", "acknowledged-icon");
  expect(request.signal.aborted).toBe(false);
  old.resolve({ ...node("one"), title: "obsolete title", icon: "obsolete-icon" });
  await loading;
  expect(workspace.breadcrumbPath()[0]?.title).toBe("acknowledged title");
  expect(workspace.breadcrumbPath()[0]?.icon).toBe("acknowledged-icon");
});

it("restarts a selected-node body read when a move is acknowledged before acceptance", async () => {
  await mount(true);
  await workspace.fetchNodeChildren("one");
  const old = body("child");
  const loading = workspace.loadNode("child", { refresh: false });
  await old.started;
  const request = requests.at(-1)!;
  replies.set("child", [Response.json({ ...node("child"), parent_id: "two" })]);
  await workspace.moveNode("child", "two");
  expect(request.signal.aborted).toBe(true);
  old.resolve({ ...node("child"), parent_id: "one" });
  expect(await loading).toBeUndefined();
  await waitFor(() => expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["two", "child"]));
});

it("shares acknowledged selected-node moves with remote hierarchy events", async () => {
  await mount(true);
  await workspace.fetchNodeChildren("one");
  const old = body("one");
  const loading = workspace.loadNode("child", { refresh: false });
  await old.started;
  const request = requests.at(-1)!;
  roots.push(Response.json({ nodes: initial() }));
  Events.current.moved("child", "two");
  await waitFor(() => expect(request.signal.aborted).toBe(true));
  old.resolve(node("one"));
  await loading;
  await waitFor(() => expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["two", "child"]));
  expect(workspace.selectedNodeData()?.parent_id).toBe("two");
});

it("keeps selected title and icon patches in the current metadata owner during ancestry", async () => {
  await mount(true);
  const old = body("one");
  const loading = workspace.loadNode("child", { refresh: false });
  await old.started;
  const request = requests.at(-1)!;
  const document = workspace.selectedDocument();
  workspace.updateNodeTitle("child", "acknowledged selected title");
  workspace.updateNodeIcon("child", "selected-icon");
  expect(request.signal.aborted).toBe(false);
  old.resolve(node("one"));
  await loading;
  expect(workspace.breadcrumbPath().at(-1)?.title).toBe("acknowledged selected title");
  expect(workspace.selectedNodeData()?.icon).toBe("selected-icon");
  expect(workspace.selectedDocument()).toBe(document);
});

it("revalidates unknown ancestry after a remote ancestor move and canonical root replacement", async () => {
  await loadDeepTree();
  replies.set("grandchild", [Response.json({ ...node("grandchild"), parent_id: "child" })]);
  const old = body("child");
  const loading = workspace.loadNode("grandchild", { refresh: false });
  await old.started;
  const request = requests.at(-1)!;
  const document = workspace.selectedDocument();
  replies.set("grandchild", [Response.json({ ...node("grandchild"), parent_id: "child" })]);
  replies.set("child", [Response.json({ ...node("child"), parent_id: "one" })]);
  replies.set("one", [Response.json({ ...node("one"), parent_id: "two" })]);
  roots.push(Response.json({ nodes: [node("two")] }));
  Events.current.moved("one", "two");
  await waitFor(() => expect(request.signal.aborted).toBe(true));
  old.resolve({ ...node("child"), parent_id: "one" });
  await loading;
  await waitFor(() =>
    expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["two", "one", "child", "grandchild"]),
  );
  expect(workspace.selectedDocument()).toBe(document);
});

it("holds replacement ancestry behind canonical roots and reads current parent metadata", async () => {
  await mount(true);
  const old = body("one");
  const loading = workspace.loadNode("child", { refresh: false });
  await old.started;
  const request = requests.at(-1)!;
  const document = workspace.selectedDocument();
  const root = Promise.withResolvers<Response>();
  roots.push(root.promise);
  replies.set("child", [Response.json({ ...node("child"), parent_id: "two", content: "must not replay" })]);
  const refresh = data.refreshRoots();
  expect(request.signal.aborted).toBe(true);
  old.resolve(node("one"));
  await loading;
  expect(requests.filter((item) => item.id === "child")).toHaveLength(1);
  root.resolve(Response.json({ nodes: initial() }));
  await refresh;
  await waitFor(() => expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["two", "child"]));
  expect(workspace.selectedNodeData()?.content).toBeUndefined();
  expect(workspace.selectedDocument()).toBe(document);
});

for (const rootOperation of ["refresh", "retry", "initial-retry"] as const) {
  it(`retires selected refresh ancestry on canonical root ${rootOperation}`, async () => {
    if (rootOperation === "initial-retry") {
      roots = [Response.json({ error: { message: "Initial roots unavailable" } }, { status: 403 })];
      await mount(false);
      await waitFor(() => expect(data.preparationError()).not.toBeNull());
      expect(data.ready()).toBe(false);
    } else await mount(true);
    await workspace.loadNode("child", { refresh: false });
    const document = workspace.selectedDocument();
    if (rootOperation === "retry") {
      roots.push(Response.json({ error: { message: "Roots unavailable" } }, { status: 403 }));
      await expect(data.refreshRoots()).rejects.toMatchObject({ status: 403 });
    }
    const old = body("one");
    const refreshing = workspace.loadNode("child", { refresh: true, canApply: () => false });
    await old.started;
    const request = requests.at(-1)!;
    const root = Promise.withResolvers<Response>();
    roots.push(root.promise);
    const moved = { ...node("child"), parent_id: "two", content: "must not replay" };
    replies.set("child", [Response.json(moved), Response.json(moved)]);
    const preparation = rootOperation === "refresh" ? data.refreshRoots() : data.retryPreparation();
    expect(request.signal.aborted).toBe(true);
    expect(request.stopped).toBe(true);
    root.resolve(Response.json({ nodes: [node("two")] }));
    await preparation;
    await waitFor(() => expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["two", "child"]));
    expect(workspace.selectedNodeData()?.parent_id).toBe("two");
    expect(workspace.selectedDocument()).toBe(document);
    expect(workspace.selectedNodeData()?.content).toBeUndefined();
    expect(data.preparationError()).toBeNull();
    old.resolve(node("one"));
    expect(await refreshing).toBeUndefined();
    expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["two", "child"]);
    expect(workspace.selectedDocument()).toBe(document);
  });
}

it("retires subscription-bound ancestry on a later canonical root refresh", async () => {
  await mount(true);
  await workspace.loadNode("child", { refresh: false });
  const document = workspace.selectedDocument();
  const old = body("one");
  roots.push(Response.json({ nodes: initial() }));
  Events.current.dispatchEvent(
    new window.MessageEvent("workspace", {
      data: JSON.stringify({ type: "resync", node_id: "0", actor_id: "0", modified: 0 }),
    }),
  );
  await old.started;
  const request = requests.at(-1)!;
  await waitFor(() => expect(data.loadingNodes()).toBe(false));
  const root = Promise.withResolvers<Response>();
  roots.push(root.promise);
  const moved = { ...node("child"), parent_id: "two", content: "must not replay" };
  replies.set("child", [Response.json(moved), Response.json(moved)]);
  const preparation = data.refreshRoots();
  expect(request.signal.aborted).toBe(true);
  expect(request.stopped).toBe(true);
  root.resolve(Response.json({ nodes: [node("two")] }));
  await preparation;
  await waitFor(() => expect(workspace.loadingNodeId()).toBeNull());
  expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["two", "child"]);
  expect(workspace.selectedDocument()).toBe(document);
  old.resolve(node("one"));
  await setImmediate();
  expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["two", "child"]);
});

it("keeps pending ancestry and its document read when lazy children confirm the same parent", async () => {
  await mount(true);
  const old = body("one");
  const loading = workspace.loadNode("child", { refresh: false });
  await old.started;
  const request = requests.at(-1)!;
  await workspace.fetchNodeChildren("one");
  expect(request.signal.aborted).toBe(false);
  expect(requests.filter((item) => item.id === "child")).toHaveLength(1);
  old.resolve(node("one"));
  await loading;
  expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["one", "child"]);
});

it("uses newly learned ancestor links instead of replaying an unknown captured parent", async () => {
  await mount(true);
  replies.set("grandchild", [Response.json({ ...node("grandchild"), parent_id: "child" })]);
  const old = body("child");
  const loading = workspace.loadNode("grandchild", { refresh: false });
  await old.started;
  childResponses.push(Response.json({ nodes: [{ ...node("child"), parent_id: "two" }] }));
  await workspace.fetchNodeChildren("two");
  replies.set("grandchild", [Response.json({ ...node("grandchild"), parent_id: "child" })]);
  replies.set("child", [Response.json({ ...node("child"), parent_id: "two" })]);
  old.resolve({ ...node("child"), parent_id: "one" });
  await loading;
  await waitFor(() =>
    expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["two", "child", "grandchild"]),
  );
});

it("revalidates ancestry after a child snapshot changes known hierarchy", async () => {
  await mount(true);
  const old = body("one");
  const loading = workspace.loadNode("child", { refresh: false });
  await old.started;
  const request = requests.at(-1)!;
  childResponses.push(Response.json({ nodes: [{ ...node("child"), parent_id: "two" }] }));
  replies.set("child", [Response.json({ ...node("child"), parent_id: "two" })]);
  await workspace.fetchNodeChildren("two");
  expect(request.signal.aborted).toBe(true);
  old.resolve(node("one"));
  await loading;
  await waitFor(() => expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["two", "child"]));
});

it("retires ancestry when authoritative metadata changes its selected parent", async () => {
  await mount(true);
  await workspace.fetchNodeChildren("one");
  const old = body("one");
  const loading = workspace.loadNode("child", { refresh: false });
  await old.started;
  const request = requests.at(-1)!;
  replies.set("child", [Response.json({ ...node("child"), parent_id: "two", modified: 2 })]);
  await metadata("child");
  await waitFor(() => expect(request.signal.aborted).toBe(true));
  old.resolve(node("one"));
  await loading;
  await waitFor(() => expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["two", "child"]));
});

it("does not restore a deleted ancestor after a retired read completes", async () => {
  await mount(true);
  const old = body("one");
  const loading = workspace.loadNode("child", { refresh: false });
  await old.started;
  const request = requests.at(-1)!;
  replies.set("child", [Response.json({ error: "deleted" }, { status: 404 })]);
  workspace.removeNode("one");
  expect(request.signal.aborted).toBe(true);
  old.resolve(node("one"));
  await loading;
  await waitFor(() => expect(workspace.failedNodeId()).toBe("child"));
  expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["child"]);
});

it("reconciles public parent patches without replacing the loaded document", async () => {
  await mount(true);
  const old = body("one");
  const loading = workspace.loadNode("child", { refresh: false });
  await old.started;
  const request = requests.at(-1)!;
  const document = workspace.selectedDocument();
  workspace.updateSelectedNode({ ...node("child"), parent_id: "two", title: "metadata only" });
  expect(request.signal.aborted).toBe(true);
  old.resolve(node("one"));
  await loading;
  await waitFor(() => expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["two", "child"]));
  expect(workspace.selectedDocument()).toBe(document);
  expect(workspace.selectedNodeData()?.title).toBe("metadata only");
});

for (const outcome of ["pending", "failed"] as const) {
  it(`resumes retired ancestry without replacing selected data after another ${outcome} target`, async () => {
    await mount(true);
    const obsoleteParent = body("one");
    const first = workspace.loadNode("child", { refresh: false });
    await obsoleteParent.started;
    const selected = { ...node("child"), parent_id: "one", title: "local metadata" };
    workspace.updateSelectedNode(selected);
    navigate("/w/workspace/two");
    await waitFor(() => expect(currentPath()).toBe("/w/workspace/two"));
    const heldTarget = transport("two");
    const other = workspace.loadNode("two", { refresh: false });
    await waitFor(() => expect(requests).toHaveLength(3));
    if (outcome === "failed") {
      heldTarget.resolve(Response.json({ error: "target unavailable" }, { status: 403 }));
      await expect(other).rejects.toMatchObject({ status: 403 });
    }
    const latestParent = body("one");
    navigate("/w/workspace/child");
    await waitFor(() => expect(currentPath()).toBe("/w/workspace/child"));
    const resumed = workspace.loadNode("child", { refresh: false });
    const latestSelected = { ...selected, title: "newer local metadata" };
    workspace.updateSelectedNode(latestSelected);
    await waitFor(() => expect(requests).toHaveLength(4));
    await latestParent.started;
    expect(requests.filter((request) => request.id === "child")).toHaveLength(1);
    expect(workspace.selectedNodeData()).toBe(latestSelected);
    latestParent.resolve(node("one"));
    await resumed;
    expect(workspace.selectedNodeData()).toBe(latestSelected);
    expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["one", "child"]);
    expect(workspace.loadError()).toBeNull();
    obsoleteParent.resolve({ ...node("one"), title: "obsolete ancestor" });
    heldTarget.resolve(Response.json(node("two")));
    await first;
    if (outcome === "pending") await other;
    expect(workspace.breadcrumbPath().map((item) => item.title)).toEqual(["one", "newer local metadata"]);
  });
}

it("keeps acknowledged metadata and document fields through a guarded hierarchy refresh", async () => {
  await mount(true);
  await workspace.loadNode("child", { refresh: false });
  const document = workspace.selectedDocument();
  const oldContent = workspace.selectedNodeData()?.content;
  const parent = body("one");
  replies.set("child", [
    Response.json({ ...node("child"), parent_id: "one", title: "Remote title", content: "Protected body" }),
  ]);
  const refresh = workspace.loadNode("child", { refresh: true, canApply: () => false });
  await parent.started;
  workspace.updateNodeTitle("child", "Acknowledged title");
  parent.resolve(node("one"));
  expect(await refresh).toBeUndefined();
  expect(workspace.selectedDocument()).toBe(document);
  expect(workspace.selectedNodeData()?.content).toBe(oldContent);
  expect(workspace.selectedNodeData()?.title).toBe("Acknowledged title");
  expect(workspace.breadcrumbPath().map((item) => item.title)).toEqual(["one", "Acknowledged title"]);
});

it("preserves the selected snapshot on read failure and retries the failed target", async () => {
  await mount(true);
  await workspace.loadNode("one", { refresh: false });
  replies.set("two", [Response.json({ error: "unavailable" }, { status: 403 })]);
  await expect(workspace.loadNode("two", { refresh: false })).rejects.toMatchObject({ status: 403 });
  expect(workspace.failedNodeId()).toBe("two");
  expect(workspace.selectedNodeId()).toBe("one");
  await workspace.loadNode("two", { refresh: false });
  expect(workspace.failedNodeId()).toBeNull();
  expect(workspace.selectedNodeId()).toBe("two");
  expect(workspace.loadError()).toBeNull();
});

it("retries a failed refresh instead of treating its retained snapshot as a successful read", async () => {
  await mount(true);
  await workspace.loadNode("one", { refresh: false });
  replies.set("one", [Response.json({ error: "unavailable" }, { status: 403 })]);
  await expect(workspace.loadNode("one", { refresh: true })).rejects.toMatchObject({ status: 403 });
  expect(workspace.failedNodeId()).toBe("one");
  expect(workspace.selectedNodeId()).toBe("one");
  replies.set("one", [Response.json({ ...node("one"), title: "recovered" })]);
  expect((await workspace.loadNode("one", { refresh: false }))?.title).toBe("recovered");
  expect(workspace.failedNodeId()).toBeNull();
  expect(workspace.loadError()).toBeNull();
});

it("keeps the cached selection when a retired JSON body completes after returning to it", async () => {
  await mount(true);
  await workspace.loadNode("one", { refresh: false });
  const cached = workspace.selectedNodeData();
  const held = body("two");
  const pending = workspace.loadNode("two", { refresh: false });
  await held.started;
  navigate("/w/workspace/one");
  await waitFor(() => expect(requests.at(-1)!.signal.aborted).toBe(true));
  expect((await workspace.loadNode("one", { refresh: false }))?.id).toBe("one");
  held.resolve(node("two"));
  expect(await pending).toBeUndefined();
  expect(workspace.selectedNodeData()).toBe(cached);
  expect(workspace.breadcrumbPath().map((item) => item.id)).toEqual(["one"]);
  expect(requests).toHaveLength(2);
});

it("retires selected-node reads on workspace replacement and provider disposal", async () => {
  await mount(true);
  const first = transport("one");
  const loading = workspace.loadNode("one", { refresh: false });
  await waitFor(() => expect(requests).toHaveLength(1));
  const request = requests[0]!;
  roots.push(Response.json({ nodes: initial() }));
  auth.setUser({ ...account, workspace_id: "replacement" });
  await waitFor(() => expect(request.signal.aborted).toBe(true));
  expect(request.stopped).toBe(true);
  expect(workspace.selectedNodeId()).toBeNull();
  expect(workspace.failedNodeId()).toBeNull();
  first.resolve(Response.json(node("one")));
  await loading;
  await waitFor(() => expect(data.ready()).toBe(true));
  const second = transport("two");
  const pending = workspace.loadNode("two", { refresh: false });
  await waitFor(() => expect(requests).toHaveLength(2));
  const retired = requests[1]!;
  cleanup();
  expect(retired.signal.aborted).toBe(true);
  expect(retired.stopped).toBe(true);
  second.resolve(Response.json(node("two")));
  await pending;
});

it("stops selected-node HTTP work while its server response remains gated", async () => {
  await mount(true);
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
    networkOrigin = `http://127.0.0.1:${address.port}/selected`;
    const pending = workspace.loadNode("one", { refresh: false });
    await waitFor(() => expect(response).not.toBeNull());
    networkOrigin = null;
    await workspace.loadNode("two", { refresh: false });
    // The server has not released its response, and test cleanup has not run.
    await waitFor(() => expect(stopped).toBe(true));
    expect(await pending).toBeUndefined();
    expect(workspace.selectedNodeId()).toBe("two");
  } finally {
    server.closeAllConnections();
    await new Promise<void>((resolve, reject) => server.close((error) => (error ? reject(error) : resolve())));
  }
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("retires same-node metadata before its transport gate opens while unrelated nodes remain live", async () => {
  await mount(true);
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
  await mount(true);
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
  await mount(true);
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
  await mount(true);
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
  await mount(true);
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
  await mount(true);
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
  await mount(true);
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
  await mount(true);
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
  await mount(true);
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
  await mount(true);
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
  await mount(true);
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
  await mount(true);
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
  await mount(true);
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
  await mount(true);
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
  await mount(true);
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

it("rejects a root refresh while canonical preparation keeps its usable snapshot and retry", async () => {
  await mount(true);
  roots.push(Response.json({ error: { message: "roots denied" } }, { status: 403 }));
  await expect(workspace.refreshRoots()).rejects.toThrow("roots denied");
  expect(data.ready()).toBe(true);
  expect(workspace.nodes.map((item) => item.id)).toEqual(["one", "two"]);
  expect(data.preparationError()).toContain("roots denied");
  expect(workspace.loadError()).toBeNull();
  roots.push(Response.json({ nodes: initial() }));
  await data.retryPreparation();
  expect(data.preparationError()).toBeNull();
});

it("rejects a selected document failure while retaining owned failure feedback", async () => {
  await mount(true);
  replies.set("one", [Response.json({ error: { message: "document denied" } }, { status: 403 })]);
  await expect(workspace.loadNode("one", { refresh: false })).rejects.toThrow("document denied");
  expect(workspace.failedNodeId()).toBe("one");
  expect(workspace.loadError()).toContain("document denied");
});

it("rejects child reads without replacing roots or publishing a second caller error", async () => {
  await mount(true);
  childResponses.push(Response.json({ error: { message: "children denied" } }, { status: 403 }));
  await expect(workspace.fetchNodeChildren("one")).rejects.toThrow("children denied");
  expect(workspace.nodes[0]?.children).toBeUndefined();
  expect(workspace.loadError()).toBeNull();
});

it("rejects an unacknowledged move without changing hierarchy", async () => {
  await mount(true);
  const fetcher = vi.mocked(globalThis.fetch).getMockImplementation();
  if (!fetcher) throw new Error("Missing base fetch fixture");
  vi.spyOn(globalThis, "fetch").mockImplementation((input, init) =>
    String(input).endsWith("/move")
      ? Promise.resolve(Response.json({ error: { message: "move denied" } }, { status: 403 }))
      : fetcher(input, init),
  );
  await expect(workspace.moveNode("one", "two")).rejects.toThrow("move denied");
  expect(workspace.nodes.map((item) => item.id)).toEqual(["one", "two"]);
  expect(workspace.saveError()).toBeNull();
});

it("rejects workspace switching for its caller to present without duplicate route feedback", async () => {
  await mount(true);
  const fetcher = vi.mocked(globalThis.fetch).getMockImplementation();
  if (!fetcher) throw new Error("Missing base fetch fixture");
  vi.spyOn(globalThis, "fetch").mockImplementation((input, init) =>
    String(input).endsWith("/switch-workspace")
      ? Promise.resolve(Response.json({ error: { message: "switch denied" } }, { status: 403 }))
      : fetcher(input, init),
  );
  await expect(workspace.switchWorkspace("other")).rejects.toThrow("switch denied");
  expect(auth.user()?.workspace_id).toBe("workspace");
  expect(workspace.switchingWorkspace()).toBe(false);
  expect(workspace.loadError()).toBeNull();
});

for (const command of ["children", "move", "switch"] as const) {
  it(`returns cancellation for a retired ${command} command even if its response fails`, async () => {
    await mount(true);
    const response = Promise.withResolvers<Response>();
    const started = Promise.withResolvers<void>();
    const fetcher = vi.mocked(globalThis.fetch).getMockImplementation();
    if (!fetcher) throw new Error("Missing base fetch fixture");
    const suffix = command === "children" ? "/nodes/one/children" : command === "move" ? "/move" : "/switch-workspace";
    vi.spyOn(globalThis, "fetch").mockImplementation((input, init) => {
      if (String(input).endsWith(suffix)) {
        started.resolve();
        return response.promise;
      }
      return fetcher(input, init);
    });
    const result =
      command === "children"
        ? workspace.fetchNodeChildren("one")
        : command === "move"
          ? workspace.moveNode("one", "two")
          : workspace.switchWorkspace("other");
    await started.promise;
    roots.push(Response.json({ nodes: initial() }));
    auth.setUser({ ...account, workspace_id: "replacement" });
    response.resolve(Response.json({ error: { message: "retired failure" } }, { status: 403 }));
    expect(await result).toBe(false);
    await waitFor(() => expect(data.loadingNodes()).toBe(false));
    expect(data.ready()).toBe(true);
    expect(data.preparationError()).toBeNull();
    expect(workspace.nodes.map((item) => item.id)).toEqual(["one", "two"]);
    expect(workspace.loadError()).toBeNull();
    expect(workspace.saveError()).toBeNull();
  });
}
