// Tests serialized canonical revisions, conflict draft retention, and awaited route/workspace/voice departure.

import { afterEach, beforeEach, it } from "node:test";
import { cleanup, render, waitFor } from "@solidjs/testing-library";
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
import { I18nProvider } from "../i18n";

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
  modified: 1,
  has_page: true,
  has_table: false,
});
let editor!: ReturnType<typeof useEditor>;
let workspace!: ReturnType<typeof useWorkspace>;
let navigation!: ReturnType<typeof useWorkspaceNavigation>;
let auth!: ReturnType<typeof useAuth>;
let navigate!: ReturnType<typeof useNavigate>;
let currentPath = "";
let calls: Array<{ body: UpdatePageRequest; resolve: (response: Response) => void }>;
let switches: number;

function Probe() {
  editor = useEditor();
  workspace = useWorkspace();
  navigation = useWorkspaceNavigation();
  auth = useAuth();
  navigate = useNavigate();
  const location = useLocation();
  navigation.registerView({
    selectedNodeId: workspace.selectedNodeId,
    selectedNodeData: workspace.selectedNodeData,
    failedNodeId: workspace.failedNodeId,
    beforeNavigate: editor.flushAutoSave,
  });
  createEffect(() => {
    currentPath = location.pathname;
  });
  createEffect(
    on(
      () => location.pathname,
      (path) => {
        const id = path.split("/").at(-1);
        if (id && path.startsWith("/w/")) void workspace.loadNode(id, { refresh: false });
      },
    ),
  );
  return null;
}
const Ready: ParentComponent = (props) => {
  const currentAuth = useAuth();
  return <Show when={currentAuth.user()}>{props.children}</Show>;
};
function mount() {
  window.history.replaceState(null, "", "/w/workspace/one");
  render(() => (
    <I18nProvider initialLocale="en">
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
  localStorage.clear();
  delete window.goModeHost;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.endsWith("/auth/me")) return Response.json(account);
    if (url.endsWith("/auth/browser/logout")) return Response.json({ ok: true });
    if (url.endsWith("/switch-workspace")) {
      switches++;
      return Response.json({ user: { ...account, workspace_id: "other" } });
    }
    if (url.endsWith("/nodes/0/children"))
      return Response.json({ nodes: [node("one", "Original", firstRevision), node("two", "Other", firstRevision)] });
    if (url.endsWith("/nodes/one/page") && init?.method === "POST")
      return new Promise<Response>((resolve) =>
        calls.push({ body: JSON.parse(String(init.body)) as UpdatePageRequest, resolve }),
      );
    if (/\/nodes\/(one|two)$/.test(url))
      return Response.json(
        node(url.endsWith("one") ? "one" : "two", url.endsWith("one") ? "Original" : "Other", firstRevision),
      );
    if (url.endsWith("/notifications/unread-count")) return Response.json({ count: 0 });
    return Response.json({ titles: {}, assets: [], history: [], nodes: [] });
  });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it("applies the canonical acknowledged title to the editor and sidebar together", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  await waitFor(() => expect(workspace.nodes.find((n) => n.id === "one")?.title).toBe("Original"));
  editor.handleTitleChange("  Canonical  ");
  const saving = editor.flushAutoSave();
  await waitFor(() => expect(calls).toHaveLength(1));
  calls[0]!.resolve(saved("Canonical", "b".repeat(64)));
  expect(await saving).toBe(true);
  expect(editor.title()).toBe("Canonical");
  expect(workspace.nodes.find((n) => n.id === "one")?.title).toBe("Canonical");
});

it("serializes saves and chooses each revision only after the preceding acknowledgement", async () => {
  mount();
  await waitFor(() => expect(editor.title()).toBe("Original"));
  editor.handleTitleChange("First");
  const first = editor.flushAutoSave();
  await waitFor(() => expect(calls).toHaveLength(1));
  editor.handleTitleChange("Second");
  const second = editor.flushAutoSave();
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
    await waitFor(() => expect(editor.saveConflict()).toBe(true));
    expect(editor.title()).toBe("Private draft");
    expect(editor.hasUnsavedChanges()).toBe(true);
    expect(workspace.selectedNodeId()).toBe("one");
    expect(currentPath).toBe("/w/workspace/one");
    expect(switches).toBe(0);
    editor.dismissExternalChange();
    expect(editor.saveConflict()).toBe(true);
    navigate("/settings");
    await waitFor(() => expect(editor.externalChange()).toBe(true));
    expect(currentPath).toBe("/w/workspace/one");
    expect(await editor.flushAutoSave()).toBe(false);
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
    expect(retired.title()).toBe("");
    expect(retired.hasUnsavedChanges()).toBe(false);
    auth.login({ ...account, session_id: "replacement" });
    await waitFor(() => expect(editor).not.toBe(retired));
    await waitFor(() => expect(editor.title()).toBe("Original"));
    calls[0]!.resolve(
      outcome === "success"
        ? saved("Private draft", "b".repeat(64))
        : Response.json({ error: { code: "CONFLICT", message: "Retired failure" } }, { status: 409 }),
    );
    await Promise.resolve();
    await Promise.resolve();
    expect(retired.title()).toBe("");
    expect(editor.title()).toBe("Original");
    expect(editor.saveConflict()).toBe(false);
    expect(currentPath).toBe("/w/workspace/one");
    expect(editor.hasUnsavedChanges()).toBe(false);
  });
}
