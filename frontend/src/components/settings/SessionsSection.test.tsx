// Tests session-list ownership when revocation completes after an account-session change.

import { afterEach, it } from "node:test";
import { batch, createSignal } from "solid-js";
import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { expect, vi } from "@tests/expect";
import type { UserResponse, SessionResponse } from "@sdk/types.gen";
import { AuthContext, type AuthContextValue } from "../../contexts/AuthContext";
import { I18nProvider } from "../../i18n";
import { createApi } from "../../useApi";
import SessionsSection from "./SessionsSection";

afterEach(cleanup);

it("a late revoke never refetches under replacement-session authority", async () => {
  const account: UserResponse = {
    id: "owner",
    email: "owner@example.com",
    name: "Owner",
    session_id: "A",
    settings: { theme: "light", language: "en" },
    created: 1,
    modified: 1,
  };
  const [user, setUser] = createSignal<UserResponse | null>(account);
  const [identity, setIdentity] = createSignal(1);
  const apiA = createApi(undefined, "A");
  const apiB = createApi(undefined, "B");
  const row: SessionResponse = {
    id: "other",
    expires_at: 2000000000,
    device_info: "Old car",
    ip_address: "",
    created: 1,
    last_used: 1,
    is_current: false,
  };
  vi.spyOn(apiA, "listSessions").mockResolvedValue({ sessions: [row] });
  const listB = vi.spyOn(apiB, "listSessions").mockResolvedValue({ sessions: [{ ...row, device_info: "New car" }] });
  const pending = Promise.withResolvers<void>();
  vi.spyOn(apiA, "revokeSession").mockImplementation(async () => {
    await pending.promise;
    return { ok: true };
  });
  const auth: AuthContextValue = {
    frontendIdentity: () => null,
    user,
    identity,
    ready: () => true,
    api: () => (user()?.session_id === "A" ? apiA : apiB),
    wsApi: () => null,
    orgApi: () => null,
    login: setUser,
    setUser,
    logout: async () => {},
    refreshUser: async () => {},
    reloadIdentity: () => {},
  };
  render(() => (
    <I18nProvider>
      <AuthContext.Provider value={auth}>
        <SessionsSection />
      </AuthContext.Provider>
    </I18nProvider>
  ));
  await waitFor(() => expect(screen.getByText("Old car")).toBeVisible());
  await waitFor(() => expect(screen.getByRole("button", { name: "Sign out session" })).toBeVisible());
  fireEvent.click(screen.getByRole("button", { name: "Sign out session" }));
  batch(() => {
    setIdentity(2);
    setUser({ ...account, session_id: "B" });
  });
  await waitFor(() => expect(screen.getByText("New car")).toBeVisible());
  pending.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(listB).toHaveBeenCalledTimes(1);
  expect(screen.queryByText("Old car")).toBeNull();
});
