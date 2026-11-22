// Tests invitation recovery, partial membership feedback, and reactive error messages.

import { afterEach, describe, it } from "node:test";
import { createSignal } from "solid-js";
import { Router, Route } from "@solidjs/router";
import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { expect, vi } from "@tests/expect";
import type { UserResponse } from "@sdk/types.gen";
import { AuthContext, type AuthContextValue } from "../contexts/AuthContext";
import { createApi, APIError } from "../useApi";
import { I18nProvider, useI18n } from "../i18n";
import AcceptInvitation from "./AcceptInvitation";

const fetchOriginal = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = fetchOriginal;
  vi.restoreAllMocks();
});

const initial: UserResponse = {
  id: "user",
  email: "user@example.test",
  name: "Invited user",
  session_id: "session",
  settings: { theme: "light", language: "en" },
  created: 1,
  modified: 1,
  organizations: [],
  workspaces: [],
};
const granted: UserResponse = {
  ...initial,
  organization_id: "org",
  workspace_id: "workspace",
  workspace_role: "ws:viewer",
  organizations: [{ id: "org-membership", user_id: "user", organization_id: "org", role: "org:member", created: 1 }],
  workspaces: [
    {
      id: "ws-membership",
      user_id: "user",
      workspace_id: "workspace",
      organization_id: "org",
      role: "ws:viewer",
      settings: { notifications: true },
      created: 1,
    },
  ],
};

function LocaleSwitch() {
  const { setLocale } = useI18n();
  return <button onClick={() => setLocale("fr")}>French</button>;
}

function mount(client: ReturnType<typeof createApi>, previewFailure: Response | null) {
  const [user, setUser] = createSignal<UserResponse | null>(initial);
  const auth: AuthContextValue = {
    user,
    identity: () => 1,
    ready: () => true,
    frontendIdentity: () => null,
    api: () => client,
    wsApi: () => null,
    orgApi: () => null,
    login: setUser,
    setUser,
    logout: async () => undefined,
    refreshUser: async () => undefined,
    reloadIdentity: () => undefined,
    refreshIdentity: async () => false,
  };
  globalThis.fetch = async () =>
    previewFailure ??
    Response.json({
      email: "",
      role: "ws:viewer",
      organization_id: "org",
      organization_name: "Shared organization",
      workspace_id: "workspace",
      workspace_name: "Shared workspace",
      expires_at: 1900000000,
    });
  window.history.replaceState({}, "", "/accept-invitation/ws?token=invitation");
  render(() => (
    <Router>
      <Route
        path="/accept-invitation/:kind"
        component={() => (
          <I18nProvider>
            <LocaleSwitch />
            <AuthContext.Provider value={auth}>
              <AcceptInvitation />
            </AuthContext.Provider>
          </I18nProvider>
        )}
      />
      <Route path="/w/*" component={() => <p>Opened workspace</p>} />
    </Router>
  ));
}

describe("AcceptInvitation", () => {
  it("translates a retained preview failure when the locale changes", async () => {
    mount(
      createApi(undefined, "session"),
      Response.json({ error: { code: "NOT_FOUND", message: "Missing invitation" } }, { status: 404 }),
    );
    await screen.findByText("This invitation is unavailable or expired.");
    fireEvent.click(screen.getByRole("button", { name: "French" }));
    await screen.findByText("Cette invitation est indisponible ou expirée.");
  });

  it("retries workspace opening without accepting an already-consumed invitation again", async () => {
    const api = createApi(undefined, "session");
    const accept = vi
      .spyOn(api, "acceptWSInvitation")
      .mockResolvedValueOnce(granted)
      .mockRejectedValue(new APIError(404, "NOT_FOUND"));
    const open = vi
      .spyOn(api, "switchWorkspace")
      .mockRejectedValueOnce(new APIError(500, "INTERNAL_ERROR"))
      .mockResolvedValue({ user: granted });
    vi.spyOn(api, "getMe").mockResolvedValue(granted);
    mount(api, null);
    fireEvent.click(await screen.findByRole("button", { name: "Join" }));
    await screen.findByText("You joined, but the workspace could not open. Try again.");
    fireEvent.click(screen.getByRole("button", { name: "Open workspace" }));
    await screen.findByText("Opened workspace");
    expect(accept).toHaveBeenCalledTimes(1);
    expect(open).toHaveBeenCalledTimes(2);
  });

  it("reconciles access after a lost acceptance response before retrying workspace opening", async () => {
    const api = createApi(undefined, "session");
    const accept = vi.spyOn(api, "acceptWSInvitation").mockRejectedValue(new TypeError("Connection lost"));
    const reconcile = vi.spyOn(api, "getMe").mockResolvedValue(granted);
    const open = vi.spyOn(api, "switchWorkspace").mockResolvedValue({ user: granted });
    mount(api, null);
    fireEvent.click(await screen.findByRole("button", { name: "Join" }));
    await screen.findByText("You joined, but the workspace could not open. Try again.");
    fireEvent.click(screen.getByRole("button", { name: "Open workspace" }));
    await screen.findByText("Opened workspace");
    expect(reconcile).toHaveBeenCalledTimes(1);
    expect(accept).toHaveBeenCalledTimes(1);
    expect(open).toHaveBeenCalledTimes(1);
  });

  it("reports a committed organization separately and retries an incomplete workspace grant", async () => {
    const api = createApi(undefined, "session");
    const partial = { ...initial, organizations: granted.organizations };
    const accept = vi
      .spyOn(api, "acceptWSInvitation")
      .mockRejectedValueOnce(
        new APIError(500, "INTERNAL_ERROR", {
          organization_id: "org",
          workspace_id: "workspace",
          organization_membership_present: true,
          workspace_membership_present: false,
          retryable: true,
          persistence_uncertain: false,
        }),
      )
      .mockResolvedValue(granted);
    vi.spyOn(api, "getMe").mockResolvedValue(partial);
    vi.spyOn(api, "switchWorkspace").mockResolvedValue({ user: granted });
    mount(api, null);
    fireEvent.click(await screen.findByRole("button", { name: "Join" }));
    await screen.findByText("You joined the organization, but workspace access is not complete. Try again.");
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    await screen.findByText("Opened workspace");
    expect(accept).toHaveBeenCalledTimes(2);
  });

  it("does not treat visible membership as confirmed saving after an explicit persistence failure", async () => {
    const api = createApi(undefined, "session");
    const accept = vi
      .spyOn(api, "acceptWSInvitation")
      .mockRejectedValueOnce(
        new APIError(500, "INTERNAL_ERROR", {
          organization_id: "org",
          workspace_id: "workspace",
          organization_membership_present: true,
          workspace_membership_present: true,
          retryable: true,
          persistence_uncertain: true,
        }),
      )
      .mockResolvedValue(granted);
    vi.spyOn(api, "getMe").mockResolvedValue(granted);
    const open = vi.spyOn(api, "switchWorkspace").mockResolvedValue({ user: granted });
    mount(api, null);
    fireEvent.click(await screen.findByRole("button", { name: "Join" }));
    await screen.findByText("Some access changes are visible, but saving could not be confirmed. Try again.");
    expect(open).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    await screen.findByText("Opened workspace");
    await waitFor(() => expect(accept).toHaveBeenCalledTimes(2));
  });
});
