// Tests authoritative initial root snapshots, route-owned initialization, and replacement readiness.

import { afterEach, beforeEach, it } from "node:test";
import { render, cleanup, waitFor } from "@solidjs/testing-library";
import { createSignal, Show } from "solid-js";
import { Router, Route, useNavigate } from "@solidjs/router";
import { HostModeProvider } from "@maruel/gomode/web/HostMode";
import type { NodeResponse, UserResponse } from "@sdk/types.gen";
import { WSRoleAdmin, WSRoleViewer } from "@sdk/types.gen";
import { expect, vi } from "@tests/expect";
import { AuthProvider, useAuth } from "./AuthContext";
import { WorkspaceDataProvider, useWorkspaceData } from "./WorkspaceDataContext";
import { WorkspaceNavigationProvider } from "./WorkspaceNavigationContext";
import { WorkspaceProvider } from "./WorkspaceContext";
import { EventSourceProvider } from "./EventSourceContext";
import { GoModeProvider, useGoMode } from "../gomode/GoMode";
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
let user = account;
let data!: ReturnType<typeof useWorkspaceData>;
let auth!: ReturnType<typeof useAuth>;
let product!: ReturnType<typeof useGoMode>;
let navigate!: ReturnType<typeof useNavigate>;
let rootResponses: Array<Promise<Response> | Response>;
let rootRequests: string[];
let welcomeResponses: Array<Promise<Response> | Response>;
let writes: string[];

function Probe() {
  data = useWorkspaceData();
  auth = useAuth();
  navigate = useNavigate();
  return null;
}
function ProductProbe() {
  product = useGoMode();
  return null;
}
function mount(path: string, toolsInitiallyMounted: boolean) {
  window.history.replaceState(null, "", path);
  const [tools, setTools] = createSignal(toolsInitiallyMounted);
  render(() => (
    <I18nProvider initialLocale="en">
      <AuthProvider>
        <Router
          root={(props) => (
            <HostModeProvider>
              <WorkspaceDataProvider>
                <WorkspaceNavigationProvider>
                  <Probe />
                  <Show when={tools()}>
                    <GoModeProvider>
                      <ProductProbe />
                      {props.children}
                    </GoModeProvider>
                  </Show>
                </WorkspaceNavigationProvider>
              </WorkspaceDataProvider>
            </HostModeProvider>
          )}
        >
          <Route path="/settings/*" component={() => null} />
          <Route
            path="/w/:wsId/*"
            component={() => (
              <EventSourceProvider>
                <WorkspaceProvider />
              </EventSourceProvider>
            )}
          />
        </Router>
      </AuthProvider>
    </I18nProvider>
  ));
  return () => setTools(true);
}
const empty = () => Response.json({ nodes: [] });
beforeEach(() => {
  user = account;
  rootResponses = [];
  rootRequests = [];
  welcomeResponses = [];
  writes = [];
  window.goModeHost = {};
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.endsWith("/auth/me")) return Response.json(user);
    if (url.endsWith("/nodes/0/children")) {
      rootRequests.push(url);
      const response = rootResponses.shift();
      if (!response) throw new Error("Unexpected root request");
      return response;
    }
    if (init?.method === "POST") {
      writes.push(url);
      const response = welcomeResponses.shift();
      if (!response) throw new Error("Unexpected provisioning write");
      return response;
    }
    throw new Error(`Unexpected request ${url}`);
  });
});
afterEach(() => {
  cleanup();
  delete window.goModeHost;
  vi.restoreAllMocks();
});

it("completes a cold-settings empty snapshot without route-owned provisioning and caches empty success", async () => {
  user = { ...account, workspace_role: WSRoleAdmin };
  rootResponses.push(empty());
  mount("/settings/user", true);
  await waitFor(() => expect(product.preparation()?.ready()).toBe(true));
  for (let i = 0; i < 3; i++) await data.loadNodes(false);
  expect(rootRequests).toHaveLength(1);
  expect(writes).toEqual([]);
});

it("registers tools before delayed empty data and accepts data before the tool owner mounts", async () => {
  const snapshot = Promise.withResolvers<Response>();
  rootResponses.push(snapshot.promise);
  mount("/settings/user", true);
  await waitFor(() => expect(rootRequests).toHaveLength(1));
  expect(product.preparation()?.ready()).toBe(false);
  snapshot.resolve(empty());
  await waitFor(() => expect(product.preparation()?.ready()).toBe(true));
  cleanup();
  rootResponses.push(empty());
  const mountTools = mount("/settings/user", false);
  await waitFor(() => expect(data.ready()).toBe(true));
  mountTools();
  await waitFor(() => expect(product.preparation()?.ready()).toBe(true));
});

it("completes an empty viewer workspace with one authoritative read", async () => {
  rootResponses.push(empty());
  mount("/w/@workspace", true);
  await waitFor(() => expect(product.preparation()?.ready()).toBe(true));
  expect(rootRequests).toHaveLength(1);
  expect(writes).toEqual([]);
});

it("waits for route-owned welcome initialization and its final canonical read", async () => {
  user = { ...account, workspace_role: WSRoleAdmin };
  const welcome = Promise.withResolvers<Response>();
  const final = Promise.withResolvers<Response>();
  rootResponses.push(empty(), final.promise);
  welcomeResponses.push(welcome.promise);
  mount("/w/@workspace", true);
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(product.preparation()?.ready()).toBe(false);
  welcome.resolve(Response.json({ id: "welcome" }));
  await waitFor(() => expect(rootRequests).toHaveLength(2));
  expect(product.preparation()?.ready()).toBe(false);
  final.resolve(Response.json({ nodes: [node("welcome")] }));
  await waitFor(() => expect(product.preparation()?.ready()).toBe(true));
});

it("recovers initial failure without resetting usable readiness for ordinary refreshes", async () => {
  rootResponses.push(Response.json({ error: { code: "NOT_FOUND", message: "Missing" } }, { status: 404 }));
  mount("/settings/user", true);
  await waitFor(() => expect(data.loadError()).not.toBeNull());
  expect(product.preparation()?.ready()).toBe(false);
  rootResponses.push(empty());
  await data.loadNodes(true);
  expect(product.preparation()?.ready()).toBe(true);
  const next = Promise.withResolvers<Response>();
  rootResponses.push(next.promise);
  const refresh = data.loadNodes(true);
  expect(product.preparation()?.ready()).toBe(true);
  next.resolve(empty());
  await refresh;
});

it("invalidates replacement workspaces and ignores old snapshots, without stranding a settings transition", async () => {
  const old = Promise.withResolvers<Response>();
  rootResponses.push(old.promise);
  mount("/settings/user", true);
  await waitFor(() => expect(rootRequests).toHaveLength(1));
  const first = product.preparation()!;
  const retiredRead = data.loadNodes(false);
  rootResponses.push(empty());
  auth.setUser({ ...account, workspace_id: "replacement" });
  await waitFor(() => expect(product.preparation()?.ready()).toBe(true));
  expect(first.ready()).toBe(false);
  old.resolve(Response.json({ nodes: [node("retired")] }));
  await retiredRead;
  expect(data.nodes).toEqual([]);
  navigate("/w/@replacement");
  expect(product.preparation()?.ready()).toBe(true);
});

it("leaving an initializing workspace releases its barrier without route-only writes on settings", async () => {
  user = { ...account, workspace_role: WSRoleAdmin };
  const snapshot = Promise.withResolvers<Response>();
  rootResponses.push(snapshot.promise);
  mount("/w/@workspace", true);
  await waitFor(() => expect(rootRequests).toHaveLength(1));
  navigate("/settings/user");
  snapshot.resolve(empty());
  await waitFor(() => expect(product.preparation()?.ready()).toBe(true));
  expect(rootRequests).toHaveLength(1);
  expect(writes).toEqual([]);
});

for (const recoveredNodes of [[], [node("welcome")]]) {
  it(`recovers a failed post-welcome final read with a successful ${recoveredNodes.length === 0 ? "empty" : "populated"} retry`, async () => {
    user = { ...account, workspace_role: WSRoleAdmin };
    rootResponses.push(
      empty(),
      Response.json({ error: { code: "NOT_FOUND", message: "Final read failed" } }, { status: 404 }),
    );
    welcomeResponses.push(Response.json({ id: "welcome" }));
    mount("/w/@workspace", true);
    await waitFor(() => {
      expect(data.loadError()).not.toBeNull();
      expect(data.loadingNodes()).toBe(false);
    });
    expect(rootRequests).toHaveLength(2);
    expect(writes).toHaveLength(1);
    expect(product.preparation()?.ready()).toBe(false);
    rootResponses.push(Response.json({ nodes: recoveredNodes }));
    await data.loadNodes(true);
    expect(data.nodes).toEqual(recoveredNodes);
    expect(data.loadError()).toBeNull();
    expect(product.preparation()?.ready()).toBe(true);
    expect(rootRequests).toHaveLength(3);
    expect(writes).toHaveLength(1);
  });
}
