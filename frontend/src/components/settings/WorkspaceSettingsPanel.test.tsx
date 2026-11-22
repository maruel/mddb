// Tests invitation ownership by the selected workspace and workspace-admin-only management.

import { afterEach, describe, it } from "node:test";
import { createSignal } from "solid-js";
import { Router, Route } from "@solidjs/router";
import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { expect, vi } from "@tests/expect";
import type { UserResponse, WorkspaceRole } from "@sdk/types.gen";
import { AuthContext, type AuthContextValue } from "../../contexts/AuthContext";
import { createApi } from "../../useApi";
import { I18nProvider } from "../../i18n";
import WorkspaceSettingsPanel from "./WorkspaceSettingsPanel";

const fetchOriginal = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = fetchOriginal;
  vi.restoreAllMocks();
});

function mount(roleB: WorkspaceRole) {
  const user: UserResponse = {
    id: "user",
    email: "user@example.test",
    name: "Member",
    created: 1,
    modified: 1,
    settings: { theme: "light", language: "en" },
    session_id: "session",
    organization_id: "org",
    org_role: "org:member",
    workspace_id: "workspace-a",
    workspace_role: "ws:admin",
    organizations: [{ id: "org-membership", user_id: "user", organization_id: "org", role: "org:member", created: 1 }],
    workspaces: ["workspace-a", "workspace-b"].map((id) => ({
      id: `${id}-membership`,
      user_id: "user",
      workspace_id: id,
      organization_id: "org",
      role: id === "workspace-b" ? roleB : "ws:admin",
      settings: { notifications: true },
      created: 1,
    })),
  };
  const [selected, setSelected] = createSignal("workspace-b");
  const api = createApi(undefined, "session");
  const auth: AuthContextValue = {
    user: () => user,
    identity: () => 1,
    ready: () => true,
    frontendIdentity: () => null,
    api: () => api,
    wsApi: () => api.ws("workspace-a"),
    orgApi: () => api.org("org"),
    login: () => undefined,
    setUser: () => undefined,
    logout: async () => undefined,
    refreshUser: async () => undefined,
    reloadIdentity: () => undefined,
    refreshIdentity: async () => false,
  };
  window.history.replaceState({}, "", "/settings/workspace/workspace-b");
  render(() => (
    <Router>
      <Route
        path="*"
        component={() => (
          <I18nProvider>
            <AuthContext.Provider value={auth}>
              <output data-testid="selected-workspace">{selected()}</output>
              <WorkspaceSettingsPanel wsId={selected()} onNavigateToOrgSettings={() => undefined} />
            </AuthContext.Provider>
          </I18nProvider>
        )}
      />
    </Router>
  ));
  return setSelected;
}

function invitation(workspace: string) {
  return {
    id: `invitation-${workspace}`,
    email: "",
    workspace_id: workspace,
    role: "ws:viewer",
    invited_by: "user",
    created: 1,
    expires_at: 1900000000,
    invite_url: `https://example.test/accept-invitation/ws?token=${workspace}`,
  };
}

describe("WorkspaceSettingsPanel", () => {
  it("can create the first invitation when the pending list is null", async () => {
    globalThis.fetch = async (input, init) => {
      const url = String(input);
      if (url.endsWith("/members")) return Response.json({ members: [] });
      if (init?.method === "POST") return Response.json(invitation("workspace-b"));
      return Response.json({ invitations: null });
    };
    mount("ws:admin");
    const create = await screen.findByRole("button", { name: "Create invite link" });
    await waitFor(() => expect(create).toBeEnabled());
    fireEvent.click(create);
    await screen.findByRole("textbox", { name: "Invite link" });
  });

  it("loads, copies and revokes selected-workspace links without organization-admin access", async () => {
    const calls: string[] = [];
    const writeText = vi.fn<(value: string) => Promise<void>>().mockResolvedValue(undefined);
    const oldClipboard = Object.getOwnPropertyDescriptor(navigator, "clipboard");
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    try {
      globalThis.fetch = async (input, init) => {
        const url = String(input);
        calls.push(`${init?.method || "GET"} ${url}`);
        if (url.endsWith("/members"))
          return Response.json({
            members: [{ id: "user", name: "Member", email: "user@example.test", role: "ws:admin" }],
          });
        if (url.endsWith("/users"))
          return Response.json({ error: { code: "FORBIDDEN", message: "Forbidden" } }, { status: 403 });
        if (url.endsWith("/delete")) return Response.json({ ok: true });
        if (url.endsWith("/invitations"))
          return Response.json({
            invitations: [invitation(url.includes("workspace-b") ? "workspace-b" : "workspace-a")],
          });
        throw new Error(`Unexpected request ${url}`);
      };
      mount("ws:admin");
      const link = await screen.findByRole("textbox", { name: "Invite link" });
      expect(link).toHaveValue("https://example.test/accept-invitation/ws?token=workspace-b");
      fireEvent.click(screen.getByRole("button", { name: "Copy invite link" }));
      await waitFor(() =>
        expect(writeText).toHaveBeenCalledWith("https://example.test/accept-invitation/ws?token=workspace-b"),
      );
      fireEvent.click(screen.getByRole("button", { name: "Revoke invite link" }));
      await waitFor(() => expect(screen.queryByRole("textbox", { name: "Invite link" })).toBeNull());
      expect(calls).toContain("POST /api/v1/workspaces/workspace-b/invitations/invitation-workspace-b/delete");
      expect(calls.some((call) => call.includes("/organizations/") || call.includes("/workspace-a/"))).toBe(false);
    } finally {
      if (oldClipboard) Object.defineProperty(navigator, "clipboard", oldClipboard);
      else Reflect.deleteProperty(navigator, "clipboard");
    }
  });

  it("creates for the selected workspace and ignores a response after the selection changes", async () => {
    const created = Promise.withResolvers<Response>();
    const calls: string[] = [];
    globalThis.fetch = async (input, init) => {
      const url = String(input);
      calls.push(`${init?.method || "GET"} ${url}`);
      if (url.endsWith("/members") || url.endsWith("/users")) return Response.json({ members: [], users: [] });
      if (init?.method === "POST") return created.promise;
      return Response.json({ invitations: [] });
    };
    const select = mount("ws:admin");
    const create = await screen.findByRole("button", { name: "Create invite link" });
    await waitFor(() => expect(create).toBeEnabled());
    fireEvent.click(create);
    await waitFor(() => expect(calls).toContain("POST /api/v1/workspaces/workspace-b/invitations"));
    select("workspace-a");
    await waitFor(() => expect(screen.getByTestId("selected-workspace")).toHaveTextContent("workspace-a"));
    created.resolve(Response.json(invitation("workspace-b")));
    await waitFor(() => expect(create).toBeEnabled());
    expect(screen.queryByDisplayValue("https://example.test/accept-invitation/ws?token=workspace-b")).toBeNull();
  });

  it("does not inherit active-workspace administrator rights for a selected viewer workspace", async () => {
    globalThis.fetch = async () => Response.json({ invitations: [], users: [] });
    mount("ws:viewer");
    await screen.findByText("Only admins can view member list.");
    expect(screen.queryByRole("button", { name: "Create invite link" })).toBeNull();
  });
});
