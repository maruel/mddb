// Tests shared workspace preparation, canonical readiness, request retirement, and stage-aware retry.

import { afterEach, beforeEach, it } from "node:test";
import { render, cleanup, waitFor } from "@solidjs/testing-library";
import { createSignal, Show } from "solid-js";
import { Router, Route, useNavigate } from "@solidjs/router";
import { HostModeProvider } from "@maruel/gomode/web/HostMode";
import type { NodeResponse, UserResponse } from "@sdk/types.gen";
import { OrgRoleOwner, WSRoleAdmin, WSRoleViewer } from "@sdk/types.gen";
import { expect, vi } from "@tests/expect";
import { AuthProvider, useAuth } from "./AuthContext";
import { WorkspaceDataProvider, useWorkspaceData } from "./WorkspaceDataContext";
import { WorkspaceNavigationProvider } from "./WorkspaceNavigationContext";
import { WorkspaceProvider, useWorkspace } from "./WorkspaceContext";
import { EventSourceProvider } from "./EventSourceContext";
import { GoModeProvider, useGoMode } from "../gomode/GoMode";
import { I18nProvider, useI18n } from "../i18n";

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
let workspace!: ReturnType<typeof useWorkspace>;
let i18n!: ReturnType<typeof useI18n>;
let auth!: ReturnType<typeof useAuth>;
let product!: ReturnType<typeof useGoMode>;
let navigate!: ReturnType<typeof useNavigate>;
let rootResponses: Array<Promise<Response> | Response>;
let rootRequests: string[];
let rootSignals: AbortSignal[];
let welcomeResponses: Array<Promise<Response> | Response>;
let welcomeSignals: AbortSignal[];
let writes: string[];

function Probe() {
  data = useWorkspaceData();
  i18n = useI18n();
  auth = useAuth();
  navigate = useNavigate();
  return null;
}
function RouteProbe() {
  workspace = useWorkspace();
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
                <WorkspaceProvider>
                  <RouteProbe />
                </WorkspaceProvider>
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
  rootSignals = [];
  welcomeResponses = [];
  welcomeSignals = [];
  writes = [];
  window.goModeHost = {};
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.endsWith("/auth/me")) return Response.json(user);
    if (url.endsWith("/nodes/0/children")) {
      rootRequests.push(url);
      if (init?.signal) rootSignals.push(init.signal);
      const response = rootResponses.shift();
      if (!response) throw new Error("Unexpected root request");
      return response;
    }
    if (init?.method === "POST") {
      writes.push(url);
      if (init.signal) welcomeSignals.push(init.signal);
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

it("defers a forced initial read until the replacement owner's dictionary is ready", async () => {
  rootResponses.push(empty());
  mount("/settings/user", true);
  await waitFor(() => expect(data.ready()).toBe(true));
  i18n.setLocale("fr");
  expect(i18n.ready()).toBe(false);
  auth.setUser({ ...account, workspace_id: "replacement" });
  rootResponses.push(empty());
  const forced = data.loadNodes(true);
  expect(rootRequests).toHaveLength(1);
  await forced;
  await waitFor(() => expect(data.ready()).toBe(true));
  expect(rootRequests).toHaveLength(2);
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

it("prepares nullable empty roots through welcome initialization and the canonical read", async () => {
  user = { ...account, workspace_role: WSRoleAdmin };
  rootResponses.push(Response.json({ nodes: null }), Response.json({ nodes: null }));
  welcomeResponses.push(Response.json({ id: "welcome" }));
  mount("/w/@workspace", true);
  await waitFor(() => expect(product.preparation()?.ready()).toBe(true));
  expect([...data.nodes]).toEqual([]);
  expect(rootRequests).toHaveLength(2);
  expect(writes).toHaveLength(1);
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
  expect(data.ready()).toBe(false);
  welcome.resolve(Response.json({ id: "welcome" }));
  await waitFor(() => expect(rootRequests).toHaveLength(2));
  expect(product.preparation()?.ready()).toBe(false);
  final.resolve(Response.json({ nodes: [node("welcome")] }));
  await waitFor(() => expect(product.preparation()?.ready()).toBe(true));
});

it("aborts an old workspace read before its held response is released", async () => {
  const old = Promise.withResolvers<Response>();
  rootResponses.push(old.promise);
  mount("/settings/user", true);
  await waitFor(() => expect(rootRequests).toHaveLength(1));
  rootResponses.push(empty());
  auth.setUser({ ...account, workspace_id: "replacement" });
  try {
    await waitFor(() => expect(product.preparation()?.ready()).toBe(true));
    await waitFor(() => expect(rootSignals[0]!.aborted).toBe(true));
  } finally {
    old.resolve(Response.json({ nodes: [node("retired")] }));
  }
  await waitFor(() => expect(data.nodes).toEqual([]));
});

it("retries a failed welcome write before publishing its final roots", async () => {
  user = { ...account, workspace_role: WSRoleAdmin };
  rootResponses.push(empty());
  welcomeResponses.push(Response.json({ error: { code: "NOT_FOUND", message: "Welcome failed" } }, { status: 404 }));
  mount("/w/@workspace", true);
  await waitFor(() => expect(data.preparationError()).not.toBeNull());
  expect(rootRequests).toHaveLength(1);
  expect(product.preparation()?.ready()).toBe(false);
  welcomeResponses.push(Response.json({ id: "welcome" }));
  rootResponses.push(Response.json({ nodes: [node("welcome")] }));
  await data.loadNodes(true);
  expect(writes).toHaveLength(2);
  expect(rootRequests).toHaveLength(2);
  expect(data.preparationError()).toBeNull();
  expect(product.preparation()?.ready()).toBe(true);
});

it("settles a failed welcome retry by reading when current permission no longer allows creation", async () => {
  user = { ...account, workspace_role: WSRoleAdmin };
  rootResponses.push(empty());
  welcomeResponses.push(Response.json({ error: { code: "FORBIDDEN", message: "Creation denied" } }, { status: 403 }));
  mount("/w/@workspace", true);
  await waitFor(() => expect(data.preparationError()).not.toBeNull());
  auth.setUser({ ...user, workspace_role: WSRoleViewer });
  rootResponses.push(empty());
  await data.loadNodes(true);
  expect(writes).toHaveLength(1);
  expect(data.preparationError()).toBeNull();
  expect(data.ready()).toBe(true);
});

for (const stage of ["provision", "roots", "welcome", "final-read"] as const) {
  it(`keeps the ${stage} preparation failure independent of route error replacement and clearing`, async () => {
    user = { ...account, workspace_role: WSRoleAdmin };
    const failure = Response.json({ error: { code: "NOT_FOUND", message: `${stage} unavailable` } }, { status: 404 });
    if (stage === "provision") {
      user = { ...account, workspace_id: undefined, workspace_role: undefined, organizations: [], workspaces: [] };
      welcomeResponses.push(failure);
    } else if (stage === "roots") rootResponses.push(failure);
    else {
      rootResponses.push(empty());
      welcomeResponses.push(stage === "welcome" ? failure : Response.json({ id: "welcome" }));
      if (stage === "final-read") rootResponses.push(failure);
    }
    mount("/w/@workspace", true);
    await waitFor(() => expect(data.preparationError()).toContain(`${stage} unavailable`));
    const message = data.preparationError();
    workspace.setLoadError("Unrelated node failure");
    expect(workspace.loadError()).toBe("Unrelated node failure");
    expect(data.preparationError()).toBe(message);
    workspace.setLoadError(null);
    expect(workspace.loadError()).toBeNull();
    expect(data.preparationError()).toBe(message);
    expect(data.ready()).toBe(false);
  });
}

it("recovers initial failure without resetting usable readiness for ordinary refreshes", async () => {
  rootResponses.push(Response.json({ error: { code: "NOT_FOUND", message: "Missing" } }, { status: 404 }));
  mount("/settings/user", true);
  await waitFor(() => expect(data.preparationError()).not.toBeNull());
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
  const retiredResult = retiredRead.catch((error: unknown) => error);
  rootResponses.push(empty());
  auth.setUser({ ...account, workspace_id: "replacement" });
  await waitFor(() => expect(product.preparation()?.ready()).toBe(true));
  expect(first.ready()).toBe(false);
  old.resolve(Response.json({ nodes: [node("retired")] }));
  expect(await retiredResult).toMatchObject({ name: "AbortError" });
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
  rootResponses.push(empty());
  navigate("/settings/user");
  try {
    await waitFor(() => expect(rootSignals[0]!.aborted).toBe(true));
    await waitFor(() => expect(product.preparation()?.ready()).toBe(true));
  } finally {
    snapshot.resolve(empty());
  }
  expect(rootRequests).toHaveLength(2);
  expect(writes).toEqual([]);
});

it("keeps settings read-only without a workspace and settles members needing access", async () => {
  user = { ...account, workspace_id: undefined, workspace_role: undefined, workspaces: [] };
  mount("/settings/user", true);
  await waitFor(() => expect(data.needsWorkspace()).toBe(true));
  expect(writes).toEqual([]);
  expect(rootRequests).toEqual([]);
  navigate("/onboarding");
  await waitFor(() => expect(data.needsWorkspace()).toBe(true));
  expect(writes).toEqual([]);
});

it("enriches cold settings on workspace entry without revoking usable readiness or recreating deleted welcome", async () => {
  user = { ...account, workspace_role: WSRoleAdmin };
  rootResponses.push(empty());
  mount("/settings/user", true);
  await waitFor(() => expect(product.preparation()?.ready()).toBe(true));
  const welcome = Promise.withResolvers<Response>();
  welcomeResponses.push(welcome.promise);
  rootResponses.push(Response.json({ nodes: [node("welcome")] }));
  navigate("/w/@workspace");
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(product.preparation()?.ready()).toBe(true);
  welcome.resolve(Response.json({ id: "welcome" }));
  await waitFor(() => expect(data.nodes).toHaveLength(1));
  rootResponses.push(empty());
  await data.loadNodes(true);
  navigate("/settings/user");
  navigate("/w/@workspace");
  expect(writes).toHaveLength(1);
  expect(rootRequests).toHaveLength(3);
  expect(product.preparation()?.ready()).toBe(true);
});

it("hands first-workspace provisioning to its successor without retiring the canonical preparation", async () => {
  user = { ...account, workspace_id: undefined, workspace_role: undefined, organizations: [], workspaces: [] };
  const original = vi.mocked(globalThis.fetch).getMockImplementation()!;
  const provisioningSignals: AbortSignal[] = [];
  vi.mocked(globalThis.fetch).mockImplementation(async (input, init) => {
    const url = String(input);
    if (init?.method === "POST" && !url.endsWith("/page/create")) {
      if (init.signal) provisioningSignals.push(init.signal);
      if (url.endsWith("/organizations")) {
        user = { ...user, organizations: [{ ...account.organizations![0]!, role: OrgRoleOwner }] };
        return Response.json({ id: "org" });
      }
      if (url.endsWith("/workspaces")) return Response.json({ id: "workspace" });
      if (url.endsWith("/switch-workspace")) {
        user = { ...account, workspace_role: WSRoleAdmin };
        return Response.json({ user });
      }
      throw new Error(`Unexpected provisioning request ${url}`);
    }
    return original(input, init);
  });
  const welcome = Promise.withResolvers<Response>();
  const canonical = Promise.withResolvers<Response>();
  rootResponses.push(empty(), canonical.promise);
  welcomeResponses.push(welcome.promise);
  mount("/onboarding", true);
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(provisioningSignals).toHaveLength(3);
  // Observe retirement directly while both successor responses remain held.
  await waitFor(() => expect(provisioningSignals.every((signal) => signal.aborted)).toBe(true));
  expect(welcomeSignals[0]!.aborted).toBe(false);
  expect(data.ready()).toBe(false);
  welcome.resolve(Response.json({ id: "welcome" }));
  await waitFor(() => expect(rootRequests).toHaveLength(2));
  expect(data.ready()).toBe(false);
  canonical.resolve(Response.json({ nodes: [node("welcome")] }));
  await waitFor(() => expect(data.ready()).toBe(true));
  expect(data.nodes[0]?.id).toBe("welcome");
  expect(welcomeSignals[0]!.aborted).toBe(false);
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
      expect(data.preparationError()).not.toBeNull();
      expect(data.loadingNodes()).toBe(false);
    });
    expect(rootRequests).toHaveLength(2);
    expect(writes).toHaveLength(1);
    expect(product.preparation()?.ready()).toBe(false);
    rootResponses.push(Response.json({ nodes: recoveredNodes }));
    await data.loadNodes(true);
    expect(data.nodes).toEqual(recoveredNodes);
    expect(data.preparationError()).toBeNull();
    expect(product.preparation()?.ready()).toBe(true);
    expect(rootRequests).toHaveLength(3);
    expect(writes).toHaveLength(1);
  });
}
