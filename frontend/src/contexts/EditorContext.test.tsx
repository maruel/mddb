// Tests serialized saves, bounded conflict recovery ownership, preserved drafts, and gated editor departure.

import { afterEach, beforeEach, it } from "node:test";
import { cleanup, render, waitFor, screen, fireEvent } from "@solidjs/testing-library";
import { createEffect, on, Show, type ParentComponent } from "solid-js";
import { Router, Route, useLocation, useNavigate } from "@solidjs/router";
import { HostModeProvider } from "@maruel/gomode/web/HostMode";
import type { NodeResponse, UpdatePageRequest, UserResponse } from "@sdk/types.gen";
import { expect, vi } from "@tests/expect";
import { AuthProvider, useAuth } from "./AuthContext";
import { WorkspaceDataProvider } from "./WorkspaceDataContext";
import { WorkspaceNavigationProvider, useWorkspaceNavigation } from "./WorkspaceNavigationContext";
import { WorkspaceProvider, useWorkspace } from "./WorkspaceContext";
import { EventSourceProvider } from "./EventSourceContext";
import { EditorProvider, useEditor } from "./EditorContext";
import { ConflictRecovery } from "../components/editor/ConflictRecovery";
import { I18nProvider } from "../i18n";
import { stripSlug } from "../utils/urls";

const firstRevision = "a".repeat(64);
const account: UserResponse = {
  id: "account",
  email: "account@example.com",
  name: "Account",
  session_id: "session",
  created: 1,
  modified: 1,
  workspace_id: "workspace",
  workspace_role: "ws:editor",
  organization_id: "org",
  organizations: [{ id: "orgmember", user_id: "account", organization_id: "org", role: "org:member", created: 1 }],
  workspaces: [
    {
      id: "membership",
      user_id: "account",
      workspace_id: "workspace",
      organization_id: "org",
      role: "ws:editor",
      settings: { notifications: true },
      created: 1,
    },
  ],
  settings: { theme: "system", language: "en" },
};
const node = (id: string, title: string, revision: string): NodeResponse => ({
  id,
  title,
  revision,
  content: "body",
  created: 1,
  modified: firstHasPage ? 1 : 0,
  has_page: firstHasPage,
  has_table: firstHasTable,
  icon: firstIcon,
});
let editor!: ReturnType<typeof useEditor>;
let workspace!: ReturnType<typeof useWorkspace>;
let navigation!: ReturnType<typeof useWorkspaceNavigation>;
let auth!: ReturnType<typeof useAuth>;
let navigate!: ReturnType<typeof useNavigate>;
let currentPath = "";
let calls: Array<{ body: UpdatePageRequest; resolve: (response: Response) => void; reject: (error: Error) => void }>;
let switches: number;
let latest: NodeResponse | null;
let latestReply: Promise<Response> | null;
let historyReply: Promise<Response> | null;
let accountLocale = "en";
let accountWorkspaceName: string;
let switchResponse: Promise<Response> | null;
let nodeResponse: Promise<Response> | null;
let firstTitle: string;
let firstIcon: string;
let firstHasPage: boolean;
let firstHasTable: boolean;

class FakeEventSource extends EventTarget {
  static current: FakeEventSource;
  onopen: ((event: Event) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  constructor(_url: string) {
    super();
    FakeEventSource.current = this;
  }
  readonly close = vi.fn();
  resync() {
    this.dispatchEvent(
      new window.MessageEvent("workspace", {
        data: JSON.stringify({ type: "resync", node_id: "0", actor_id: "0", modified: 0 }),
      }),
    );
  }
  metadata() {
    this.dispatchEvent(
      new window.MessageEvent("workspace", {
        data: JSON.stringify({
          type: "node_updated",
          node_id: "one",
          actor_id: "other",
          modified: 1,
          revision: firstRevision,
          aspects: ["metadata"],
        }),
      }),
    );
  }
}

function Probe() {
  editor = useEditor();
  workspace = useWorkspace();
  navigation = useWorkspaceNavigation();
  auth = useAuth();
  navigate = useNavigate();
  const location = useLocation();
  navigation.registerView({
    beginNavigation: editor.beginIntent,
    selectedNodeId: workspace.selectedNodeId,
    selectedNodeData: workspace.selectedNodeData,
    failedNodeId: workspace.failedNodeId,
    beforeNavigate: async () => (await editor.prepareDeparture()) !== null,
  });
  createEffect(() => {
    currentPath = location.pathname;
  });
  createEffect(
    on(
      () => location.pathname,
      (path) => {
        const id = path.split("/").at(-1);
        if (id && path.startsWith("/w/")) void workspace.loadNode(stripSlug(id), { refresh: false, navigation: true });
      },
    ),
  );
  return <ConflictRecovery onSaved={() => undefined} />;
}
const Ready: ParentComponent = (props) => {
  const currentAuth = useAuth();
  return <Show when={currentAuth.user()}>{props.children}</Show>;
};
function mount(locale?: "en" | "fr" | "de" | "es") {
  accountLocale = locale ?? "en";
  window.history.replaceState(null, "", "/w/workspace/one");
  render(() => (
    <I18nProvider initialLocale={locale ?? "en"}>
      <AuthProvider>
        <Router
          root={(props) => (
            <HostModeProvider>
              <WorkspaceDataProvider>
                <WorkspaceNavigationProvider>
                  <Ready>
                    <EventSourceProvider>
                      <WorkspaceProvider>
                        <EditorProvider>
                          <Probe />
                          {props.children}
                        </EditorProvider>
                      </WorkspaceProvider>
                    </EventSourceProvider>
                  </Ready>
                </WorkspaceNavigationProvider>
              </WorkspaceDataProvider>
            </HostModeProvider>
          )}
        >
          <Route path="/settings" component={() => null} />
          <Route path="/w/:wsId/:id" component={() => null} />
        </Router>
      </AuthProvider>
    </I18nProvider>
  ));
}
const saved = (title: string, revision: string) => Response.json({ node: node("one", title, revision) });
beforeEach(() => {
  calls = [];
  switches = 0;
  latest = null;
  latestReply = null;
  historyReply = null;
  switchResponse = null;
  nodeResponse = null;
  accountWorkspaceName = "Original workspace";
  firstTitle = "Original";
  firstIcon = "";
  firstHasPage = true;
  firstHasTable = false;
  localStorage.clear();
  delete window.goModeHost;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.endsWith("/auth/me"))
      return Response.json({
        ...account,
        workspace_name: accountWorkspaceName,
        settings: { ...account.settings, language: accountLocale },
      });
    if (url.endsWith("/auth/browser/logout")) return Response.json({ ok: true });
    if (url.endsWith("/switch-workspace")) {
      switches++;
      return switchResponse ?? Response.json({ user: { ...account, workspace_id: "other" } });
    }
    if (url.endsWith("/nodes/0/children"))
      return Response.json({ nodes: [node("one", "Original", firstRevision), node("two", "Other", firstRevision)] });
    if (url.endsWith("/nodes/one/page") && init?.method === "POST")
      return new Promise<Response>((resolve, reject) =>
        calls.push({ body: JSON.parse(String(init.body)) as UpdatePageRequest, resolve, reject }),
      );
    if (url.endsWith("/nodes/one") && latestReply) return latestReply;
    if (url.includes("/nodes/one/history") && historyReply) return historyReply;
    if (url.endsWith("/nodes/one") && latest) return Response.json(latest);
    if (/\/nodes\/(one|two)$/.test(url))
      return (
        (url.endsWith("two") ? nodeResponse : null) ??
        Response.json(
          node(url.endsWith("one") ? "one" : "two", url.endsWith("one") ? firstTitle : "Other", firstRevision),
        )
      );
    if (url.endsWith("/notifications/unread-count")) return Response.json({ count: 0 });
    return Response.json({ titles: {}, assets: [], history: [], nodes: [] });
  });
});

for (const field of ["title", "content"] as const) {
  it(`becomes clean when ${field} returns to its acknowledged value`, async () => {
    mount();
    await waitFor(() => expect(editor.title()).toBe("Original"));
    if (field === "title") {
      editor.handleTitleChange("Edited");
      editor.handleTitleChange("Original");
    } else {
      editor.handleContentChange("Edited body");
      editor.handleContentChange("body");
    }
    expect(editor.hasUnsavedChanges()).toBe(false);
    expect(await editor.beginIntent().drain()).toBe(true);
    expect(calls).toHaveLength(0);
    navigate("/settings");
    await waitFor(() => expect(currentPath).toBe("/settings"));
  });
}

it("title undo becomes clean and redo restores the dirty draft", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  editor.handleTitleChange("Edited");
  await editor.undo();
  expect(editor.title()).toBe("Original");
  expect(editor.hasUnsavedChanges()).toBe(false);
  await editor.redo();
  expect(editor.title()).toBe("Edited");
  expect(editor.hasUnsavedChanges()).toBe(true);
});

it("saves a reverted clean draft after the pending different write is acknowledged", async () => {
  vi.stubGlobal("EventSource", FakeEventSource);
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  editor.handleTitleChange("Pending B");
  const saving = editor.beginIntent().drain();
  await waitFor(() => expect(calls).toHaveLength(1));
  editor.handleTitleChange("Original");
  expect(editor.hasUnsavedChanges()).toBe(false);
  firstTitle = "Unacknowledged remote title";
  FakeEventSource.current.resync();
  await new Promise<void>((resolve) => setImmediate(resolve));
  expect(editor.title()).toBe("Original");
  expect(editor.externalChange()).toBe(true);
  navigate("/settings");
  expect(currentPath).toBe("/w/workspace/one");
  calls[0]!.resolve(saved("Pending B", "b".repeat(64)));
  await waitFor(() => expect(calls).toHaveLength(2));
  expect(calls[1]!.body).toEqual({ title: "Original", content: "body", expected_revision: "b".repeat(64) });
  expect(currentPath).toBe("/w/workspace/one");
  calls[1]!.resolve(saved("Original", "c".repeat(64)));
  await saving;
  await waitFor(() => expect(currentPath).toBe("/settings"));
});

for (const outcome of ["conflict", "unknown"] as const) {
  it(`retains ${outcome} protection after reverting to acknowledged text`, async () => {
    vi.stubGlobal("EventSource", FakeEventSource);
    mount();
    await waitFor(() => expect(editor.title()).toBe("Original"));
    editor.handleTitleChange("Pending B");
    const saving = editor.beginIntent().drain();
    await waitFor(() => expect(calls).toHaveLength(1));
    editor.handleTitleChange("Original");
    if (outcome === "unknown") calls[0]!.reject(new TypeError("Response lost after commit"));
    else calls[0]!.resolve(Response.json({ error: { code: "CONFLICT", message: "Changed" } }, { status: 409 }));
    expect(await saving).toBe(false);
    expect(editor.hasUnsavedChanges()).toBe(false);
    expect(editor.saveIssue()).toBe(outcome);
    editor.handleTitleChange("Changed again");
    editor.handleTitleChange("Original");
    expect(await editor.beginIntent().drain()).toBe(false);
    firstTitle = "Remote C";
    FakeEventSource.current.resync();
    await new Promise<void>((resolve) => setImmediate(resolve));
    expect(editor.title()).toBe("Original");
    expect(editor.saveIssue()).toBe(outcome);
    navigate("/settings");
    expect(currentPath).toBe("/w/workspace/one");
    expect(calls).toHaveLength(1);
  });
}

it("keeps unchanged uploads admitted until their application settles", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  let finish!: (value: boolean) => void;
  const upload = editor.runAsyncEdit(
    () =>
      new Promise<boolean>((resolve) => {
        finish = resolve;
      }),
  );
  await waitFor(() => expect(finish).toBeDefined());
  expect(editor.asyncEditPending()).toBe(true);
  expect(editor.hasUnsavedChanges()).toBe(false);
  navigate("/settings");
  expect(currentPath).toBe("/w/workspace/one");
  expect(calls).toHaveLength(0);
  finish(true);
  await upload;
  await waitFor(() => expect(currentPath).toBe("/settings"));
  expect(calls).toHaveLength(0);
});

it("writes an explicitly recovered old baseline against the latest remote revision", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  await conflictDraft();
  latest = { ...node("one", "Remote C", "c".repeat(64)), content: "Remote body" };
  await editor.compareChanges();
  const saving = editor.saveRecovery(editor.recovery()!, { title: "Original", content: "body" });
  await waitFor(() => expect(calls).toHaveLength(2));
  expect(editor.hasUnsavedChanges()).toBe(false);
  expect(calls[1]!.body).toEqual({ title: "Original", content: "body", expected_revision: latest.revision });
  calls[1]!.resolve(saved("Original", "d".repeat(64)));
  expect(await saving).toBe(true);
  expect(editor.saveIssue()).toBe(null);
});

it("keeps manual comparison admission visible and blocked after a clean revert", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  editor.handleTitleChange("Review draft");
  await editor.compareChanges();
  editor.closeRecovery();
  editor.handleTitleChange("Original");
  expect(editor.hasUnsavedChanges()).toBe(false);
  expect(editor.saveIssue()).toBe(null);
  expect(editor.comparisonBlocked()).toBe(true);
  expect(editor.autoSaveStatus()).toBe("idle");
  expect(workspace.saveError()).toBe(null);
  navigate("/settings");
  expect(await editor.beginIntent().drain()).toBe(false);
  expect(currentPath).toBe("/w/workspace/one");
  expect(calls).toHaveLength(0);
  await editor.compareChanges();
  expect(editor.recovery()?.local).toEqual({ title: "Original", content: "body" });
});

it("retains comparison admission after an older write succeeds and becomes clean", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  editor.handleTitleChange("Pending draft");
  const saving = editor.beginIntent().drain();
  await waitFor(() => expect(calls).toHaveLength(1));
  const comparing = editor.compareChanges();
  calls[0]!.resolve(saved("Pending draft", "b".repeat(64)));
  await saving;
  await comparing;
  expect(editor.hasUnsavedChanges()).toBe(false);
  expect(editor.saveIssue()).toBe(null);
  expect(editor.comparisonBlocked()).toBe(true);
  expect(editor.autoSaveStatus()).toBe("idle");
  editor.closeRecovery();
  navigate("/settings");
  expect(await editor.beginIntent().drain()).toBe(false);
  expect(calls).toHaveLength(1);
  expect(currentPath).toBe("/w/workspace/one");
});

it("keeps an uncertain save outcome for explicit recovery rather than repeating it", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  editor.handleTitleChange("Uncertain draft");
  const saving = editor.beginIntent().drain();
  await waitFor(() => expect(calls).toHaveLength(1));
  calls[0]!.reject(new TypeError("Response lost after the server committed"));
  expect(await saving).toBe(false);
  expect(editor.saveIssue()).toBe("unknown");
  expect(editor.hasUnsavedChanges()).toBe(true);
  expect(editor.title()).toBe("Uncertain draft");
  expect(await editor.beginIntent().drain()).toBe(false);
  expect(calls).toHaveLength(1);
});

for (const state of ["clean", "dirty", "pending"] as const) {
  it(`resynchronizes a ${state} editor without discarding private work`, async () => {
    vi.stubGlobal("EventSource", FakeEventSource);
    mount();
    await waitFor(() => expect(editor.title()).toBe("Original"));
    if (state !== "clean") editor.handleTitleChange("Private draft");
    let saving: Promise<boolean> | null = null;
    if (state === "pending") {
      saving = editor.beginIntent().drain();
      await waitFor(() => expect(calls).toHaveLength(1));
    }
    firstTitle = "Fresh remote title";
    FakeEventSource.current.resync();
    if (state === "clean") {
      await waitFor(() => expect(editor.title()).toBe("Fresh remote title"));
      expect(editor.externalChange()).toBe(false);
    } else {
      await waitFor(() => expect(editor.externalChange()).toBe(true));
      expect(editor.title()).toBe("Private draft");
      if (saving) {
        calls[0]!.resolve(saved("Private draft", "b".repeat(64)));
        await saving;
      }
    }
  });
}

for (const state of ["clean", "dirty", "pending"] as const) {
  it(`applies authoritative metadata at the same timestamp with a ${state} draft`, async () => {
    vi.stubGlobal("EventSource", FakeEventSource);
    mount();
    await waitFor(() => expect(editor.title()).toBe("Original"));
    if (state !== "clean") editor.handleTitleChange("Private draft");
    let saving: Promise<boolean> | null = null;
    if (state === "pending") {
      saving = editor.beginIntent().drain();
      await waitFor(() => expect(calls).toHaveLength(1));
    }
    firstIcon = "🚀";
    FakeEventSource.current.metadata();
    if (state === "clean") {
      await waitFor(() => expect(editor.icon()).toBe("🚀"));
      expect(editor.title()).toBe("Original");
    } else {
      await waitFor(() => expect(editor.externalChange()).toBe(true));
      expect(editor.title()).toBe("Private draft");
      expect(editor.icon()).toBe("");
      if (saving) {
        calls[0]!.resolve(saved("Private draft", "b".repeat(64)));
        await saving;
      }
    }
  });
}

for (const state of ["clean", "dirty", "pending"] as const) {
  it(`applies hybrid metadata withdrawal with a ${state} draft`, async () => {
    vi.stubGlobal("EventSource", FakeEventSource);
    firstIcon = "🚀";
    firstHasTable = true;
    mount();
    await waitFor(() => expect(editor.title()).toBe("Original"));
    if (state !== "clean") editor.handleTitleChange("Private draft");
    let saving: Promise<boolean> | null = null;
    if (state === "pending") {
      saving = editor.beginIntent().drain();
      await waitFor(() => expect(calls).toHaveLength(1));
    }
    firstHasPage = false;
    firstIcon = "";
    FakeEventSource.current.metadata();
    await waitFor(() => expect(workspace.nodes[0]!.has_page).toBe(false));
    expect(workspace.nodes[0]!.icon).toBe("");
    if (state === "clean") {
      await waitFor(() => expect(editor.icon()).toBe(""));
      expect(editor.title()).toBe("Original");
    } else {
      await waitFor(() => expect(editor.externalChange()).toBe(true));
      expect(editor.title()).toBe("Private draft");
      expect(editor.icon()).toBe("🚀");
      if (saving) {
        calls[0]!.resolve(saved("Private draft", "b".repeat(64)));
        await saving;
      }
    }
  });
}

for (const openBeforeReturn of [false, true]) {
  it(`synchronizes the active document when first open occurs ${openBeforeReturn ? "before" : "after"} returning from a canceled destination`, async () => {
    vi.stubGlobal("EventSource", FakeEventSource);
    const destination = Promise.withResolvers<Response>();
    nodeResponse = destination.promise;
    mount();
    await waitFor(() => expect(editor.title()).toBe("Original"));
    navigate("/w/workspace/two");
    await waitFor(() => expect(workspace.loadingNodeId()).toBe("two"));
    firstTitle = "Fresh active page";
    if (openBeforeReturn) FakeEventSource.current.onopen?.(new Event("open"));
    navigate("/w/workspace/one");
    await waitFor(() => expect(currentPath).toBe("/w/workspace/one"));
    if (!openBeforeReturn) FakeEventSource.current.onopen?.(new Event("open"));
    await waitFor(() => expect(editor.title()).toBe("Fresh active page"));
    destination.resolve(Response.json(node("two", "Canceled destination", firstRevision)));
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    expect(workspace.selectedNodeId()).toBe("one");
    expect(workspace.loadingNodeId()).toBeNull();
    expect(editor.editingLocked()).toBe(false);
  });
}

it("retains a running first-open refresh when navigation interrupts it and returns", async () => {
  vi.stubGlobal("EventSource", FakeEventSource);
  const destination = Promise.withResolvers<Response>();
  const interrupted = Promise.withResolvers<Response>();
  let reads = 0;
  let returned = false;
  const fetch = vi.mocked(globalThis.fetch).getMockImplementation()!;
  vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
    if (String(input).endsWith("/nodes/one")) {
      reads++;
      if (returned) return Promise.resolve(Response.json(node("one", "Fresh returned page", firstRevision)));
      if (reads > 1) return interrupted.promise.then((response) => response.clone());
    }
    return fetch(input, init);
  });
  nodeResponse = destination.promise;
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  FakeEventSource.current.onopen?.(new Event("open"));
  await waitFor(() => expect(workspace.loadingNodeId()).toBe("one"));
  navigate("/w/workspace/two");
  await waitFor(() => expect(workspace.loadingNodeId()).toBe("two"));
  returned = true;
  navigate("/w/workspace/one");
  await waitFor(() => expect(editor.title()).toBe("Fresh returned page"));
  interrupted.resolve(Response.json(node("one", "Obsolete interrupted refresh", firstRevision)));
  destination.resolve(Response.json(node("two", "Canceled destination", firstRevision)));
  await new Promise<void>((resolve) => setTimeout(resolve, 0));
  expect(editor.title()).toBe("Fresh returned page");
  expect(workspace.selectedNodeId()).toBe("one");
});

for (const replacement of ["destination", "account"] as const) {
  it(`retires the selected document's deferred refresh after ${replacement} replacement`, async () => {
    vi.stubGlobal("EventSource", FakeEventSource);
    const destination = Promise.withResolvers<Response>();
    let reads = 0;
    const fetch = vi.mocked(globalThis.fetch).getMockImplementation()!;
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      if (String(input).endsWith("/nodes/one")) reads++;
      if (String(input).endsWith("/nodes/two")) return destination.promise.then((response) => response.clone());
      return fetch(input, init);
    });
    mount();
    await waitFor(() => expect(editor.title()).toBe("Original"));
    navigate("/w/workspace/two");
    await waitFor(() => expect(workspace.loadingNodeId()).toBe("two"));
    FakeEventSource.current.onopen?.(new Event("open"));
    if (replacement === "destination") {
      destination.resolve(Response.json(node("two", "Other", firstRevision)));
      await waitFor(() => expect(editor.title()).toBe("Other"));
    } else {
      auth.setUser({ ...account, workspace_id: "replacement" });
      await waitFor(() => expect(workspace.selectedNodeId()).toBeNull());
    }
    expect(reads).toBe(1);
    firstTitle = "Fresh replacement read";
    navigate(replacement === "destination" ? "/w/workspace/one" : "/w/replacement/one");
    await waitFor(() => expect(editor.title()).toBe("Fresh replacement read"));
    destination.resolve(Response.json(node("two", "Retired destination", firstRevision)));
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    expect(editor.title()).toBe("Fresh replacement read");
    expect(reads).toBe(2);
  });
}

it("retains the deferred refresh's draft guard after returning to its document", async () => {
  vi.stubGlobal("EventSource", FakeEventSource);
  const destination = Promise.withResolvers<Response>();
  const refreshing = Promise.withResolvers<Response>();
  let reads = 0;
  const fetch = vi.mocked(globalThis.fetch).getMockImplementation()!;
  vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
    if (String(input).endsWith("/nodes/one") && ++reads > 1) return refreshing.promise;
    return fetch(input, init);
  });
  nodeResponse = destination.promise;
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  navigate("/w/workspace/two");
  await waitFor(() => expect(workspace.loadingNodeId()).toBe("two"));
  FakeEventSource.current.onopen?.(new Event("open"));
  navigate("/w/workspace/one");
  await waitFor(() => expect(reads).toBe(2));
  editor.handleContentChange("Private returned draft");
  refreshing.resolve(Response.json(node("one", "Fresh remote title", firstRevision)));
  await waitFor(() => expect(editor.externalChange()).toBe(true));
  expect(editor.content()).toBe("Private returned draft");
  expect(editor.title()).toBe("Original");
  await waitFor(() => expect(workspace.breadcrumbPath().at(-1)?.title).toBe("Fresh remote title"));
  destination.resolve(Response.json(node("two", "Canceled destination", firstRevision)));
});

it("resumes the active document's deferred refresh when a workspace switch fails", async () => {
  vi.stubGlobal("EventSource", FakeEventSource);
  const switching = Promise.withResolvers<Response>();
  switchResponse = switching.promise;
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  const switched = workspace.switchWorkspace("other");
  await waitFor(() => expect(workspace.switchingWorkspace()).toBe(true));
  firstTitle = "Fresh after failed switch";
  FakeEventSource.current.onopen?.(new Event("open"));
  switching.reject(new TypeError("Switch connection lost"));
  await switched;
  await waitFor(() => expect(editor.title()).toBe("Fresh after failed switch"));
  expect(workspace.selectedNodeId()).toBe("one");
});

it("retains complete dirty breadcrumbs when ancestor refresh fails and retries successfully", async () => {
  vi.stubGlobal("EventSource", FakeEventSource);
  const failing = Promise.withResolvers<Response>();
  let refreshing = false;
  let parentReads = 0;
  const warning = vi.spyOn(console, "warn").mockImplementation(() => undefined);
  const fetch = vi.mocked(globalThis.fetch).getMockImplementation()!;
  vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
    const id = String(input).split("/").at(-1);
    if (id === "one")
      return Promise.resolve(Response.json({ ...node("one", "Original", firstRevision), parent_id: "parent" }));
    if (id === "parent") {
      parentReads++;
      return refreshing
        ? failing.promise
        : Promise.resolve(Response.json(node("parent", "Complete parent", firstRevision)));
    }
    return fetch(input, init);
  });
  mount();
  await waitFor(() => expect(workspace.breadcrumbPath()).toHaveLength(2));
  editor.handleContentChange("Private body");
  refreshing = true;
  FakeEventSource.current.onopen?.(new Event("open"));
  await waitFor(() => expect(parentReads).toBe(2));
  failing.reject(new TypeError("Ancestor connection lost"));
  await waitFor(() => expect(warning).toHaveBeenCalledWith("Breadcrumb refresh failed", expect.any(TypeError)));
  expect(workspace.loadError()).toContain("Ancestor connection lost");
  expect(workspace.breadcrumbPath().map((entry) => entry.title)).toEqual(["Complete parent", "Original"]);
  expect(editor.content()).toBe("Private body");
  refreshing = false;
  FakeEventSource.current.resync();
  await waitFor(() => expect(parentReads).toBe(3));
  expect(workspace.breadcrumbPath()).toHaveLength(2);
  expect(editor.hasUnsavedChanges()).toBe(true);
});

it("keeps first-open refresh subordinate to an admitted destination and edits that destination", async () => {
  vi.stubGlobal("EventSource", FakeEventSource);
  const destination = Promise.withResolvers<Response>();
  let destinationReads = 0;
  const fetch = vi.mocked(globalThis.fetch).getMockImplementation()!;
  vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
    if (String(input).endsWith("/nodes/two")) {
      destinationReads++;
      return destinationReads === 1
        ? destination.promise
        : Promise.resolve(Response.json(node("two", "Other", firstRevision)));
    }
    if (String(input).endsWith("/nodes/two/page") && init?.method === "POST")
      return Promise.resolve(Response.json({ node: node("two", "Destination draft", "b".repeat(64)) }));
    return fetch(input, init);
  });
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  navigate("/w/workspace/two");
  await waitFor(() => expect(workspace.loadingNodeId()).toBe("two"));
  FakeEventSource.current.onopen?.(new Event("open"));
  expect(editor.editingLocked()).toBe(true);
  destination.resolve(Response.json(node("two", "Other", firstRevision)));
  await waitFor(() => expect(editor.title()).toBe("Other"));
  expect(currentPath).toBe("/w/workspace/two");
  expect(workspace.selectedNodeId()).toBe("two");
  expect(destinationReads).toBe(2);
  expect(editor.editingLocked()).toBe(false);
  editor.handleTitleChange("Destination draft");
  const saving = editor.beginIntent().drain();
  await waitFor(() =>
    expect(
      vi
        .mocked(globalThis.fetch)
        .mock.calls.some(
          ([url, init]) => String(url).endsWith("/nodes/two/page") && String(init?.body).includes("Destination draft"),
        ),
    ).toBe(true),
  );
  expect(await saving).toBe(true);
});

for (const { pendingPath, dirty } of [
  { pendingPath: true, dirty: true },
  { pendingPath: false, dirty: true },
  { pendingPath: true, dirty: false },
]) {
  it(`rebuilds the entire ${pendingPath ? "pending" : "loaded"} breadcrumb after first open with a ${dirty ? "dirty" : "clean"} editor`, async () => {
    vi.stubGlobal("EventSource", FakeEventSource);
    const top = Promise.withResolvers<Response>();
    let moved = false;
    let topReads = 0;
    const fetch = vi.mocked(globalThis.fetch).getMockImplementation()!;
    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const id = String(input).split("/").at(-1);
      if (id === "one")
        return Promise.resolve(
          Response.json({
            ...node("one", "Original", firstRevision),
            content: moved ? "Fresh body" : "body",
            parent_id: "middle",
          }),
        );
      if (id === "middle")
        return Promise.resolve(
          Response.json({
            ...node("middle", moved ? "Renamed middle" : "Old middle", firstRevision),
            parent_id: moved ? "0" : "top",
          }),
        );
      if (id === "top") {
        topReads++;
        return top.promise;
      }
      return fetch(input, init);
    });
    mount();
    await waitFor(() => expect(topReads).toBe(1));
    if (!pendingPath) {
      top.resolve(Response.json(node("top", "Old top", firstRevision)));
      await waitFor(() => expect(workspace.breadcrumbPath()).toHaveLength(3));
    }
    if (dirty) editor.handleContentChange("Private body");
    moved = true;
    FakeEventSource.current.onopen?.(new Event("open"));
    top.resolve(Response.json(node("top", "Old top", firstRevision)));
    await waitFor(() =>
      expect(workspace.breadcrumbPath().map((entry) => entry.title)).toEqual(["Renamed middle", "Original"]),
    );
    expect(editor.content()).toBe(dirty ? "Private body" : "Fresh body");
    expect(editor.hasUnsavedChanges()).toBe(dirty);
  });
}

for (const target of ["node", "workspace"] as const) {
  it(`locks the old editor while a delayed ${target} replacement is pending`, async () => {
    mount();
    await waitFor(() => expect(editor.title()).toBe("Original"));
    let release!: (response: Response) => void;
    const delayed = new Promise<Response>((resolve) => {
      release = resolve;
    });
    let departing: Promise<void> | undefined;
    if (target === "node") {
      nodeResponse = delayed;
      navigate("/w/workspace/two");
      await waitFor(() => expect(workspace.loadingNodeId()).toBe("two"));
    } else {
      switchResponse = delayed;
      departing = workspace.switchWorkspace("other");
      await waitFor(() => expect(workspace.switchingWorkspace()).toBe(true));
    }
    expect(editor.editingLocked()).toBe(true);
    editor.handleTitleChange("Too late");
    editor.handleContentChange("Too late");
    expect(editor.title()).toBe("Original");
    expect(editor.content()).toBe("body");
    expect(editor.hasUnsavedChanges()).toBe(false);
    release(
      target === "node"
        ? Response.json(node("two", "Other", firstRevision))
        : Response.json({ user: { ...account, workspace_id: "other" } }),
    );
    if (departing) await departing;
    await waitFor(() => expect(editor.editingLocked()).toBe(false));
  });
}

it("locks the selected deletion without consuming undo while its request is pending", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  editor.handleTitleChange("Before delete");
  const saving = editor.beginIntent().drain();
  await waitFor(() => expect(calls).toHaveLength(1));
  calls[0]!.resolve(saved("Before delete", "b".repeat(64)));
  expect(await saving).toBe(true);
  const admitted = editor.beginIntent();
  workspace.setDeletingNodeId("one");
  expect(editor.editingLocked()).toBe(true);
  expect(await editor.prepareDeparture()).toBeNull();
  expect(admitted.current()).toBe(true);
  editor.handleContentChange("Too late");
  await editor.undo();
  expect(editor.content()).toBe("body");
  expect(editor.title()).toBe("Before delete");
  workspace.setDeletingNodeId(null);
  await editor.undo();
  expect(editor.title()).toBe("Original");
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("applies the canonical acknowledged title to the editor and sidebar together", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  await waitFor(() => expect(workspace.nodes.find((n) => n.id === "one")?.title).toBe("Original"));
  editor.handleTitleChange("  Canonical  ");
  const saving = editor.beginIntent().drain();
  await waitFor(() => expect(calls).toHaveLength(1));
  calls[0]!.resolve(saved("Canonical", "b".repeat(64)));
  expect(await saving).toBe(true);
  expect(editor.title()).toBe("Canonical");
  expect(workspace.nodes.find((n) => n.id === "one")?.title).toBe("Canonical");
});

it("derives cleanliness from a canonical acknowledgement matching newer local edits", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  editor.handleTitleChange("  Canonical  ");
  const saving = editor.beginIntent().drain();
  await waitFor(() => expect(calls).toHaveLength(1));
  editor.handleTitleChange("Canonical");
  editor.handleContentChange("Canonical body");
  calls[0]!.resolve(
    Response.json({ node: { ...node("one", "Canonical", "b".repeat(64)), content: "Canonical body" } }),
  );
  expect(await saving).toBe(true);
  expect(editor.title()).toBe("Canonical");
  expect(editor.content()).toBe("Canonical body");
  expect(editor.hasUnsavedChanges()).toBe(false);
  expect(calls).toHaveLength(1);
});

it("serializes saves and chooses each revision only after the preceding acknowledgement", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  editor.handleTitleChange("First");
  const intent = editor.beginIntent();
  const first = intent.drain();
  await waitFor(() => expect(calls).toHaveLength(1));
  editor.handleTitleChange("Second");
  const second = intent.drain();
  expect(calls).toHaveLength(1);
  expect(calls[0]?.body.expected_revision).toBe(firstRevision);
  calls[0]!.resolve(saved("First", "b".repeat(64)));
  await waitFor(() => expect(calls).toHaveLength(2));
  expect(calls[1]?.body).toEqual({ title: "Second", content: "body", expected_revision: "b".repeat(64) });
  calls[1]!.resolve(saved("Second", "c".repeat(64)));
  expect(await first).toBe(true);
  expect(await second).toBe(true);
  expect(editor.title()).toBe("Second");
  expect(editor.hasUnsavedChanges()).toBe(false);
});

it("shows saved feedback only while the latest intended draft remains acknowledged", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  editor.handleTitleChange("Saved draft");
  const saving = editor.beginIntent().drain();
  await waitFor(() => expect(calls).toHaveLength(1));
  calls[0]!.resolve(saved("Saved draft", "b".repeat(64)));
  expect(await saving).toBe(true);
  expect(editor.autoSaveStatus()).toBe("saved");
  editor.handleTitleChange("New unsaved draft");
  expect(editor.autoSaveStatus()).toBe("idle");
  expect(editor.hasUnsavedChanges()).toBe(true);
});

it("preserves an unsaved draft and undo across a same-session account refresh", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  editor.handleTitleChange("Private title");
  editor.handleContentChange("Private body");
  const loaded = workspace.selectedNodeData();
  const identity = auth.identity();
  await auth.refreshUser();
  expect(auth.identity()).toBe(identity);
  expect(workspace.selectedNodeData()).toBe(loaded);
  expect(editor.title()).toBe("Private title");
  expect(editor.content()).toBe("Private body");
  expect(editor.hasUnsavedChanges()).toBe(true);
  expect(editor.canUndo()).toBe(true);
  await editor.undo();
  expect(editor.title()).toBe("Original");
  expect(editor.content()).toBe("Private body");
  expect(editor.hasUnsavedChanges()).toBe(true);
});

it("preserves queued drafts and acknowledged revisions across same-session refreshes", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  editor.handleTitleChange("First draft");
  const saving = editor.beginIntent().drain();
  await waitFor(() => expect(calls).toHaveLength(1));
  editor.handleTitleChange("Second draft");
  await auth.refreshUser();
  expect(editor.title()).toBe("Second draft");
  expect(editor.hasUnsavedChanges()).toBe(true);
  expect(editor.canUndo()).toBe(true);
  calls[0]!.resolve(saved("First draft", "b".repeat(64)));
  await waitFor(() => expect(calls).toHaveLength(2));
  expect(calls[1]!.body).toEqual({ title: "Second draft", content: "body", expected_revision: "b".repeat(64) });
  calls[1]!.resolve(saved("Second draft", "c".repeat(64)));
  expect(await saving).toBe(true);
  await auth.refreshUser();
  expect(editor.title()).toBe("Second draft");
  expect(editor.hasUnsavedChanges()).toBe(false);
  expect(editor.autoSaveStatus()).toBe("saved");
  editor.handleTitleChange("Third draft");
  const next = editor.beginIntent().drain();
  await waitFor(() => expect(calls).toHaveLength(3));
  expect(calls[2]!.body.expected_revision).toBe("c".repeat(64));
  calls[2]!.resolve(saved("Third draft", "d".repeat(64)));
  expect(await next).toBe(true);
});

it("uses the current workspace name when an in-flight save acknowledges after a rename", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  navigate("/w/@workspace+original-workspace/@one+original");
  await waitFor(() => expect(currentPath).toBe("/w/@workspace+original-workspace/@one+original"));
  editor.handleTitleChange("Saved draft");
  const saving = editor.beginIntent().drain();
  await waitFor(() => expect(calls).toHaveLength(1));
  const loaded = workspace.selectedNodeData();
  const identity = auth.identity();
  accountWorkspaceName = "Renamed workspace";
  await auth.refreshUser();
  expect(auth.identity()).toBe(identity);
  expect(workspace.selectedNodeData()).toBe(loaded);
  expect(editor.title()).toBe("Saved draft");
  expect(editor.hasUnsavedChanges()).toBe(true);
  expect(editor.canUndo()).toBe(true);
  calls[0]!.resolve(saved("Saved draft", "b".repeat(64)));
  expect(await saving).toBe(true);
  expect(window.location.pathname).toBe("/w/@workspace+renamed-workspace/@one+saved-draft");
  expect(editor.hasUnsavedChanges()).toBe(false);
});

it("retires an in-flight save when the same node is authoritatively reloaded", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  editor.handleTitleChange("Older pending draft");
  const saving = editor.beginIntent().drain();
  await waitFor(() => expect(calls).toHaveLength(1));
  workspace.setSelectedNodeData(node("one", "Reloaded title", "c".repeat(64)));
  await waitFor(() => expect(editor.title()).toBe("Reloaded title"));
  calls[0]!.resolve(saved("Older pending draft", "b".repeat(64)));
  expect(await saving).toBe(false);
  expect(editor.title()).toBe("Reloaded title");
  expect(editor.hasUnsavedChanges()).toBe(false);
  expect(editor.autoSaveStatus()).toBe("idle");
  editor.handleTitleChange("Draft after reload");
  const current = editor.beginIntent().drain();
  await waitFor(() => expect(calls).toHaveLength(2));
  expect(calls[1]!.body.expected_revision).toBe("c".repeat(64));
  calls[1]!.resolve(saved("Draft after reload", "d".repeat(64)));
  expect(await current).toBe(true);
});

it("retires asynchronous edit admission with its loaded document", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  let finish!: (applied: boolean) => void;
  const afterSave = vi.fn();
  const pending = editor.runAsyncEdit(() => new Promise<boolean>((resolve) => (finish = resolve)), afterSave);
  await waitFor(() => expect(editor.asyncEditPending()).toBe(true));
  workspace.setSelectedNodeData(node("one", "Reloaded title", "c".repeat(64)));
  await waitFor(() => expect(editor.title()).toBe("Reloaded title"));
  expect(editor.asyncEditPending()).toBe(false);
  finish(true);
  await pending;
  expect(afterSave).not.toHaveBeenCalled();
  expect(calls).toHaveLength(0);
  expect(editor.title()).toBe("Reloaded title");
});

it("keeps admitted history across an authoritative refresh of its target document", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  let reply!: (response: Response) => void;
  historyReply = new Promise<Response>((resolve) => (reply = resolve));
  const intent = editor.beginIntent();
  const loading = editor.loadHistory("one", intent);
  workspace.setSelectedNodeData(node("one", "Reloaded title", "c".repeat(64)));
  await waitFor(() => expect(editor.title()).toBe("Reloaded title"));
  const commits = [
    { hash: "commit", message: "Saved", author_name: "User", author_email: "user@example.com", timestamp: 1 },
  ];
  reply(Response.json({ history: commits }));
  await loading;
  expect(intent.current()).toBe(true);
  expect(editor.showHistory()).toBe(true);
  expect(editor.history()?.commits).toEqual(commits);
});

it("allows the next queued draft to retry after an ordinary save failure", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  editor.handleTitleChange("First draft");
  const intent = editor.beginIntent();
  const first = intent.drain();
  await waitFor(() => expect(calls).toHaveLength(1));
  editor.handleTitleChange("Retry draft");
  const retry = intent.drain();
  calls[0]!.resolve(Response.json({ error: "invalid draft" }, { status: 422 }));
  expect(await first).toBe(false);
  await waitFor(() => expect(calls).toHaveLength(2));
  expect(calls[1]!.body).toEqual({ title: "Retry draft", content: "body", expected_revision: firstRevision });
  expect(editor.autoSaveStatus()).toBe("saving");
  calls[1]!.resolve(saved("Retry draft", "b".repeat(64)));
  expect(await retry).toBe(true);
  expect(workspace.saveError()).toBe(null);
  expect(editor.hasUnsavedChanges()).toBe(false);
  expect(editor.autoSaveStatus()).toBe("saved");
});

it("keeps comparison admission blocked when an older save fails ordinarily", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  editor.handleTitleChange("Pending draft");
  const intent = editor.beginIntent();
  const first = intent.drain();
  await waitFor(() => expect(calls).toHaveLength(1));
  editor.handleTitleChange("Newer queued draft");
  const queued = intent.drain();
  const comparing = editor.compareChanges();
  expect(editor.saveIssue()).toBe(null);
  expect(editor.comparisonBlocked()).toBe(true);
  calls[0]!.resolve(Response.json({ error: "invalid draft" }, { status: 422 }));
  expect(await first).toBe(false);
  expect(editor.saveIssue()).toBe(null);
  expect(editor.comparisonBlocked()).toBe(true);
  expect(await queued).toBe(false);
  await comparing;
  expect(editor.recovery()?.local.title).toBe("Newer queued draft");
  editor.closeRecovery();
  navigate("/settings");
  expect(await editor.beginIntent().drain()).toBe(false);
  expect(calls).toHaveLength(1);
  expect(editor.title()).toBe("Newer queued draft");
  expect(editor.hasUnsavedChanges()).toBe(true);
  expect(currentPath).toBe("/w/workspace/one");
});

it("resumes only the latest departure after the pending draft is acknowledged", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  editor.handleTitleChange("Departing draft");
  navigate("/settings");
  await waitFor(() => expect(calls).toHaveLength(1));
  navigate("/w/workspace/two");
  expect(currentPath).toBe("/w/workspace/one");
  calls[0]!.resolve(saved("Departing draft", "b".repeat(64)));
  await waitFor(() => expect(currentPath).toBe("/w/workspace/two"));
  expect(calls).toHaveLength(1);
  expect(editor.title()).toBe("Other");
});

for (const target of ["node", "settings", "workspace", "voice", "back"] as const) {
  it(`retains a conflicted draft and blocks ${target} departure`, async () => {
    mount();
    await waitFor(() => expect(editor.title()).toBe("Original"));
    if (target === "back") {
      navigate("/w/workspace/two");
      await waitFor(() => expect(workspace.selectedNodeId()).toBe("two"));
      navigate("/w/workspace/one");
      await waitFor(() => expect(workspace.selectedNodeId()).toBe("one"));
    }
    editor.handleTitleChange("Private draft");
    let departure: Promise<unknown> | undefined;
    if (target === "workspace") departure = workspace.switchWorkspace("other");
    else if (target === "voice")
      departure = navigation
        .focusNode("two", { sessionID: "voice", signal: new AbortController().signal, isCurrent: () => true })
        .catch((error) => error);
    else navigate(target === "settings" ? "/settings" : target === "back" ? -1 : "/w/workspace/two");
    await waitFor(() => expect(calls).toHaveLength(1));
    expect(workspace.selectedNodeId()).toBe("one");
    expect(currentPath).toBe("/w/workspace/one");
    calls[0]!.resolve(Response.json({ error: { code: "CONFLICT", message: "Changed" } }, { status: 409 }));
    if (departure) await departure;
    await waitFor(() => expect(editor.saveIssue()).toBe("conflict"));
    expect(editor.title()).toBe("Private draft");
    expect(editor.hasUnsavedChanges()).toBe(true);
    expect(workspace.selectedNodeId()).toBe("one");
    expect(currentPath).toBe("/w/workspace/one");
    expect(switches).toBe(0);
    editor.dismissExternalChange();
    expect(editor.saveIssue()).toBe("conflict");
    navigate("/settings");
    await waitFor(() => expect(editor.externalChange()).toBe(true));
    expect(currentPath).toBe("/w/workspace/one");
    expect(await editor.beginIntent().drain()).toBe(false);
    expect(calls).toHaveLength(1);
  });
}

for (const outcome of ["success", "conflict"] as const) {
  it(`retires private drafts and ignores late ${outcome} after owner replacement`, async () => {
    mount();
    await waitFor(() => expect(editor.title()).toBe("Original"));
    editor.handleTitleChange("Private draft");
    navigate("/w/workspace/two");
    await waitFor(() => expect(calls).toHaveLength(1));
    const retired = editor;
    await auth.logout();
    expect(retired.beginIntent().current()).toBe(false);
    expect(retired.title()).toBe("");
    expect(retired.hasUnsavedChanges()).toBe(false);
    auth.login({ ...account, session_id: "replacement" });
    await waitFor(() => expect(editor).not.toBe(retired));
    await waitFor(() => expect(editor.title()).toBe("Original"));
    expect(retired.beginIntent().current()).toBe(false);
    calls[0]!.resolve(
      outcome === "success"
        ? saved("Private draft", "b".repeat(64))
        : Response.json({ error: { code: "CONFLICT", message: "Retired failure" } }, { status: 409 }),
    );
    await Promise.resolve();
    await Promise.resolve();
    expect(retired.title()).toBe("");
    expect(editor.title()).toBe("Original");
    expect(editor.saveIssue()).toBeNull();
    expect(currentPath).toBe("/w/workspace/one");
    expect(editor.hasUnsavedChanges()).toBe(false);
  });
}

it("comparison includes an admitted asynchronous insertion before capturing its draft", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  await conflictDraft();
  latest = node("one", "Saved title", "b".repeat(64));
  let finish!: () => void;
  const gate = new Promise<void>((resolve) => {
    finish = resolve;
  });
  const editing = editor.runAsyncEdit(async () => {
    await gate;
    editor.handleContentChange("Inserted reference");
    return true;
  });
  const comparing = editor.compareChanges();
  try {
    // All mocked HTTP replies settle within this event-loop turn. The admitted
    // insertion remains held until this test explicitly releases it.
    await new Promise<void>((resolve) => setImmediate(resolve));
    expect(editor.recovery()).toBe(null);
  } finally {
    finish();
    await editing;
    await comparing;
  }
  expect(editor.recovery()?.local.content).toBe("Inserted reference");
});

it("recovery saving waits for admitted insertions and supersedes their navigation continuation", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  await conflictDraft();
  latest = node("one", "Saved title", "b".repeat(64));
  await editor.compareChanges();
  const session = editor.recovery()!;
  let finish!: () => void;
  const gate = new Promise<void>((resolve) => {
    finish = resolve;
  });
  const editing = editor.runAsyncEdit(
    async () => {
      await gate;
      editor.handleContentChange("Inserted while reviewing");
      return true;
    },
    async (intent) => {
      intent.navigate("/settings");
    },
  );
  const saving = editor.saveRecovery(session, { title: "Reviewed", content: "Reviewed body" });
  try {
    await new Promise<void>((resolve) => setImmediate(resolve));
    expect(calls).toHaveLength(1);
  } finally {
    vi.spyOn(globalThis, "fetch").mockImplementation(async () => saved("Reviewed", "c".repeat(64)));
    finish();
    calls[1]?.resolve(saved("Reviewed", "c".repeat(64)));
    await Promise.all([editing, saving]);
  }
  expect(await saving).toBe(false);
  expect(editor.content()).toBe("Inserted while reviewing");
  expect(editor.recovery()?.local.content).toBe("Inserted while reviewing");
  expect(currentPath).toBe("/w/workspace/one");
});

it("an uncertain recovery save retains unknown-outcome protection", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  await conflictDraft();
  latest = node("one", "Saved title", "b".repeat(64));
  await editor.compareChanges();
  const saving = editor.saveRecovery(editor.recovery()!, { title: "Reviewed", content: "Reviewed body" });
  await waitFor(() => expect(calls).toHaveLength(2));
  calls[1]!.reject(new Error("reply lost"));
  expect(await saving).toBe(false);
  expect(editor.saveIssue()).toBe("unknown");
  expect(await editor.beginIntent().drain()).toBe(false);
  expect(calls).toHaveLength(2);
});

for (const failure of ["http", "network", "unversioned"] as const) {
  it(`retains an uncertain recovery outcome through a failed ${failure} comparison retry`, async () => {
    mount();
    await waitFor(() => expect(editor.title()).toBe("Original"));
    await conflictDraft();
    latest = node("one", "Saved title", "b".repeat(64));
    await editor.compareChanges();
    const saving = editor.saveRecovery(editor.recovery()!, { title: "Reviewed", content: "Reviewed body" });
    await waitFor(() => expect(calls).toHaveLength(2));
    calls[1]!.reject(new Error("save reply lost"));
    expect(await saving).toBe(false);
    let failRead!: () => void;
    latestReply = new Promise<Response>((resolve, reject) => {
      failRead = () => {
        if (failure === "http") resolve(Response.json({ error: "read forbidden" }, { status: 403 }));
        else if (failure === "unversioned")
          resolve(Response.json({ ...latest, has_page: false, has_table: true, revision: undefined }));
        else reject(new Error("read reply lost"));
      };
    });
    fireEvent.click(screen.getByRole("button", { name: "Load latest version" }));
    await new Promise<void>((resolve) => setImmediate(resolve));
    const issueWhileReading = editor.saveIssue();
    failRead();
    await waitFor(() => expect(editor.recoveryLoading()).toBe(false));
    expect(issueWhileReading).toBe("unknown");
    expect(editor.recoveryError()).toContain("Could not load the latest version");
    expect(editor.saveIssue()).toBe("unknown");
    expect(editor.title()).toBe("Reviewed");
    expect(editor.content()).toBe("Reviewed body");
    expect(await editor.beginIntent().drain()).toBe(false);
    expect(calls).toHaveLength(2);
    latestReply = null;
    latest = { ...node("one", "Reviewed", "c".repeat(64)), content: "Reviewed body" };
    fireEvent.click(screen.getByRole("button", { name: "Load latest version" }));
    await waitFor(() => expect(editor.recovery()?.saved.revision).toBe(latest!.revision));
    expect(editor.saveIssue()).toBe("conflict");
    expect(calls).toHaveLength(2);
  });
}

it("a canceled latest read cannot resolve an uncertain recovery outcome", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  await conflictDraft();
  latest = node("one", "Saved title", "b".repeat(64));
  await editor.compareChanges();
  const saving = editor.saveRecovery(editor.recovery()!, { title: "Reviewed", content: "Reviewed body" });
  await waitFor(() => expect(calls).toHaveLength(2));
  calls[1]!.reject(new Error("save reply lost"));
  expect(await saving).toBe(false);
  let reply!: (response: Response) => void;
  latestReply = new Promise<Response>((resolve) => {
    reply = resolve;
  });
  fireEvent.click(screen.getByRole("button", { name: "Load latest version" }));
  await new Promise<void>((resolve) => setImmediate(resolve));
  editor.closeRecovery();
  reply(Response.json(node("one", "Saved title", "c".repeat(64))));
  await new Promise<void>((resolve) => setImmediate(resolve));
  expect(editor.saveIssue()).toBe("unknown");
  expect(editor.recovery()).toBe(null);
  expect(editor.recoveryLoading()).toBe(false);
  expect(calls).toHaveLength(2);
});

async function conflictDraft() {
  editor.handleTitleChange("Draft");
  const saving = editor.beginIntent().drain();
  await waitFor(() => expect(calls).toHaveLength(1));
  calls[0]!.resolve(Response.json({ error: "conflict" }, { status: 409 }));
  expect(await saving).toBe(false);
}

it("combines an independent saved body with a draft title and saves against the fetched revision", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  await conflictDraft();
  latest = { ...node("one", "Original", "b".repeat(64)), content: "saved body" };
  await editor.compareChanges();
  const session = editor.recovery()!;
  expect(session.base).toEqual({ title: "Original", content: "body" });
  expect(session.titleMerge.parts.map((part) => ("text" in part ? part.text : "CONFLICT"))).toEqual(["Draft"]);
  expect(session.contentMerge.parts.map((part) => ("text" in part ? part.text : "CONFLICT"))).toEqual(["saved body"]);
  expect(editor.content()).toBe("body");
  const saving = editor.saveRecovery(session, { title: "Draft", content: "saved body" });
  await waitFor(() => expect(calls).toHaveLength(2));
  expect(calls[1]!.body).toEqual({ title: "Draft", content: "saved body", expected_revision: latest.revision });
  calls[1]!.resolve(Response.json({ node: { ...latest, title: "Draft", revision: "c".repeat(64) } }));
  expect(await saving).toBe(true);
  expect(editor.content()).toBe("saved body");
  expect(editor.hasUnsavedChanges()).toBe(false);
  expect(editor.recovery()).toBe(null);
});

it("incorporates edits during latest fetch and recomputes instead of discarding later local edits", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  await conflictDraft();
  let reply!: (response: Response) => void;
  latestReply = new Promise((resolve) => {
    reply = resolve;
  });
  const comparing = editor.compareChanges();
  editor.handleContentChange("typed while loading");
  reply(Response.json(node("one", "Saved title", "b".repeat(64))));
  await comparing;
  const session = editor.recovery()!;
  expect(session.local.content).toBe("typed while loading");
  editor.handleTitleChange("new keystrokes");
  expect(await editor.saveRecovery(session, { title: "obsolete", content: "obsolete" })).toBe(false);
  expect(editor.title()).toBe("new keystrokes");
  expect(editor.content()).toBe("typed while loading");
  expect(editor.recovery()?.local.title).toBe("new keystrokes");
  expect(editor.recovery()?.previousResult).toEqual({ title: "obsolete", content: "obsolete" });
  expect(screen.getByRole("textbox", { name: /Previous recovery result/ })).toHaveValue("# obsolete\n\nobsolete");
  expect(editor.recoveryError()).toContain("draft changed");
  expect(calls).toHaveLength(1);
});

it("retains edits during recovery saves and the acknowledged canonical base for subsequent comparisons", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  await conflictDraft();
  latest = node("one", "Saved title", "b".repeat(64));
  await editor.compareChanges();
  const saving = editor.saveRecovery(editor.recovery()!, { title: "  Recovered  ", content: "body" });
  await waitFor(() => expect(calls).toHaveLength(2));
  editor.handleContentChange("new body");
  calls[1]!.resolve(saved("Recovered", "c".repeat(64)));
  expect(await saving).toBe(true);
  expect(workspace.saveError()).toBe(null);
  expect(editor.content()).toBe("new body");
  expect(editor.hasUnsavedChanges()).toBe(true);
  await editor.compareChanges();
  expect(editor.recovery()?.base).toEqual({ title: "Recovered", content: "body" });
  expect(editor.recovery()?.local.content).toBe("new body");
});

it("preserves a recovered draft after another writer wins again and requires a new comparison", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  await conflictDraft();
  latest = node("one", "Saved title", "b".repeat(64));
  await editor.compareChanges();
  const saving = editor.saveRecovery(editor.recovery()!, { title: "Reviewed draft", content: "reviewed body" });
  await waitFor(() => expect(calls).toHaveLength(2));
  calls[1]!.resolve(Response.json({ error: "conflict" }, { status: 409 }));
  expect(await saving).toBe(false);
  expect(editor.title()).toBe("Reviewed draft");
  expect(editor.content()).toBe("reviewed body");
  expect(editor.saveIssue()).toBe("conflict");
  expect(editor.recovery()).toBe(null);
  latest = { ...latest, content: "even newer", revision: "d".repeat(64) };
  await editor.compareChanges();
  expect(editor.recovery()?.saved.content).toBe("even newer");
  expect(editor.recovery()?.base.title).toBe("Original");
});

it("ignores retired latest responses without exposing the previous account draft", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  await conflictDraft();
  const retired = editor;
  let reply!: (response: Response) => void;
  latestReply = new Promise((resolve) => {
    reply = resolve;
  });
  const comparing = editor.compareChanges();
  await auth.logout();
  await waitFor(() => expect(retired.title()).toBe(""));
  reply(Response.json(node("one", "Private saved title", "b".repeat(64))));
  await comparing;
  expect(retired.recovery()).toBe(null);
  expect(retired.recoveryError()).toBe(null);
  expect(retired.recoveryLoading()).toBe(false);
});

for (const [locale, compare, save] of [
  ["en", "Compare changes", "Save recovered page"],
  ["fr", "Comparer les modifications", "Enregistrer la page récupérée"],
  ["de", "Änderungen vergleichen", "Wiederhergestellte Seite speichern"],
  ["es", "Comparar cambios", "Guardar página recuperada"],
] as const) {
  it(`presents translated explicit overlap choices in ${locale}`, async () => {
    mount(locale);
    await waitFor(() => expect(editor.title()).toBe("Original"));
    await conflictDraft();
    latest = node("one", "Different title", "b".repeat(64));
    await editor.compareChanges();
    await waitFor(() => expect(screen.getByRole("dialog", { name: compare })).toBeTruthy());
    const saving = screen.getByRole("button", { name: save });
    expect(saving).toBeDisabled();
    expect(screen.getAllByRole("radio")).toHaveLength(2);
    fireEvent.click(screen.getAllByRole("radio")[0]!);
    expect(saving).not.toBeDisabled();
    expect(screen.getAllByRole("textbox")[0]).toHaveValue("Draft");
    if (locale === "en") {
      fireEvent.input(screen.getAllByRole("textbox")[0]!, { target: { value: "Manually reviewed title" } });
      fireEvent.click(screen.getAllByRole("radio")[1]!);
      expect(screen.getByRole("textbox", { name: /Previous recovery result/ })).toHaveValue(
        "# Manually reviewed title\n\nbody",
      );
    }
  });
}

it("converts latest relative links for review and writes the reviewed result in canonical disk form", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  await conflictDraft();
  latest = { ...node("one", "Original", "b".repeat(64)), content: "[linked](../TARGET/index.md)" };
  await editor.compareChanges();
  const session = editor.recovery()!;
  expect(session.saved.content).toBe("[linked](/w/@workspace/@TARGET)");
  const saving = editor.saveRecovery(session, { title: "  Reviewed  ", content: session.saved.content });
  await waitFor(() => expect(calls).toHaveLength(2));
  expect(calls[1]!.body.content).toBe("[linked](../TARGET/index.md)");
  calls[1]!.resolve(Response.json({ node: { ...latest, title: "Reviewed", revision: "c".repeat(64) } }));
  expect(await saving).toBe(true);
  expect(editor.title()).toBe("Reviewed");
  expect(editor.content()).toBe("[linked](/w/@workspace/@TARGET)");
});

it("cancels pending comparisons and ignores their eventual response", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  await conflictDraft();
  let reply!: (response: Response) => void;
  latestReply = new Promise((resolve) => {
    reply = resolve;
  });
  const comparing = editor.compareChanges();
  editor.closeRecovery();
  reply(Response.json(node("one", "Late version", "b".repeat(64))));
  await comparing;
  expect(editor.recovery()).toBe(null);
  expect(editor.recoveryLoading()).toBe(false);
  expect(editor.title()).toBe("Draft");
});
it(
  "does not let a late voice lookup supersede a newer departure waiting for an insertion",
  { timeout: 7000 },
  async () => {
    mount();
    await waitFor(() => expect(editor.title()).toBe("Original"));
    let reply!: (response: Response) => void;
    nodeResponse = new Promise<Response>((resolve) => {
      reply = resolve;
    });
    const focus = navigation
      .focusNode("two", {
        sessionID: "voice",
        signal: new AbortController().signal,
        isCurrent: () => true,
      })
      .catch((error: unknown) => error);
    let finish!: () => void;
    const insertion = new Promise<void>((resolve) => {
      finish = resolve;
    });
    const editing = editor.runAsyncEdit(async () => {
      await insertion;
      editor.handleContentChange("Inserted reference");
      return true;
    });
    navigate("/settings");
    expect(currentPath).toBe("/w/workspace/one");
    reply(Response.json(node("two", "Other", firstRevision)));
    // Let the real lookup and its continuation run while the insertion is held.
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    finish();
    await editing;
    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0]?.body.content).toBe("Inserted reference");
    calls[0]!.resolve(saved("Original", "b".repeat(64)));
    await waitFor(() => expect(currentPath).toBe("/settings"));
    await focus;
  },
);
