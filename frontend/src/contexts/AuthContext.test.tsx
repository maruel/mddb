// Tests the identity reconciliation policy, in-place cookie identity refresh, and the reload and logout-recovery fallbacks.

import { afterEach, beforeEach, it } from "node:test";
import { cleanup, render, waitFor } from "@solidjs/testing-library";
import { expect, vi } from "@tests/expect";
import { sessionFromMe } from "@tests/session";
import { APIError } from "@sdk/api.gen";
import type { UserResponse } from "@sdk/types.gen";
import { I18nProvider } from "../i18n";
import { AuthProvider, reconcileIdentity, useAuth } from "./AuthContext";

const account = {
  id: "account-a",
  email: "account-a@example.com",
  name: "Account A",
  session_id: "session-a",
  settings: {},
} as UserResponse;
const messages: Record<string, unknown>[] = [];
// Queued /auth/me responses: served to identity probes in the order they arrive.
let responses: Array<Response | Error> = [];
// Queued /auth/browser/logout responses.
let logoutResponses: Array<Response | Error> = [];
let auth: ReturnType<typeof useAuth> | null = null;

function nextResponse(queue: Array<Response | Error>): Response {
  const response = queue.shift();
  if (response instanceof Error) throw response;
  if (!response) throw new Error("No cookie identity response queued");
  return response;
}

function currentAuth(): ReturnType<typeof useAuth> {
  if (auth === null) throw new Error("Auth context did not render");
  return auth;
}

function Probe() {
  auth = useAuth();
  return null;
}

function mount() {
  render(() => (
    <I18nProvider initialLocale="en">
      <AuthProvider>
        <Probe />
      </AuthProvider>
    </I18nProvider>
  ));
}

beforeEach(() => {
  messages.length = 0;
  responses = [];
  logoutResponses = [];
  window.goModeFrontendDocument = Promise.resolve("auth-document");
  window.gomodeAuth = {
    postMessage(message) {
      const { frontendDocument, frontendRequest, ...authFields } = JSON.parse(message) as Record<string, unknown>;
      expect(frontendDocument).toBe("auth-document");
      expect(typeof frontendRequest).toBe("string");
      messages.push(authFields);
    },
  };
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    if (url === "/api/v1/auth/browser/logout") return nextResponse(logoutResponses);
    if (url === "/api/v1/auth/session") return sessionFromMe(nextResponse(responses));
    expect(url).toBe("/api/v1/auth/me");
    return nextResponse(responses);
  });
});

afterEach(() => {
  cleanup();
  auth = null;
  delete window.goModeFrontendDocument;
  delete window.gomodeAuth;
  vi.restoreAllMocks();
});

it("refreshes a same-session account without native transitions", async () => {
  responses.push(Response.json(account));
  mount();
  await waitFor(() => expect(currentAuth().ready()).toBe(true));
  const previousIdentity = currentAuth().identity();
  const published = messages.length;
  responses.push(Response.json({ ...account, name: "Account A renamed" }));

  await expect(currentAuth().refreshIdentity()).resolves.toBe(true);
  expect(currentAuth().ready()).toBe(true);
  expect(currentAuth().user()?.name).toBe("Account A renamed");
  expect(currentAuth().identity()).toBe(previousIdentity);
  expect(messages).toHaveLength(published);
});

it("refreshes a replacement session in place", async () => {
  responses.push(Response.json(account));
  mount();
  await waitFor(() => expect(currentAuth().ready()).toBe(true));
  const previousIdentity = currentAuth().identity();
  const published = messages.length;
  responses.push(Response.json({ ...account, session_id: "session-b" }));

  await expect(currentAuth().refreshIdentity()).resolves.toBe(true);
  expect(currentAuth().ready()).toBe(true);
  expect(currentAuth().user()?.session_id).toBe("session-b");
  expect(currentAuth().identity()).toBeGreaterThan(previousIdentity);
  expect(messages.slice(published)).toEqual([{ authChanging: true }, { authChanged: true, nativeAccess: true }]);
});

it("reloads when the cookie identity changes accounts", async () => {
  responses.push(Response.json(account));
  mount();
  await waitFor(() => expect(currentAuth().ready()).toBe(true));
  const published = messages.length;
  responses.push(Response.json({ ...account, id: "account-b", session_id: "session-b" }));

  await expect(currentAuth().refreshIdentity()).resolves.toBe(false);
  expect(currentAuth().ready()).toBe(false);
  expect(currentAuth().user()).toBeNull();
  expect(messages.slice(published)).toEqual([{ authChanging: true }]);
});

it("reloads when the refreshed identity is missing", async () => {
  responses.push(Response.json(account));
  mount();
  await waitFor(() => expect(currentAuth().ready()).toBe(true));
  const published = messages.length;
  responses.push(Response.json({ error: { code: "NOT_FOUND", message: "missing" } }, { status: 404 }));

  await expect(currentAuth().refreshIdentity()).resolves.toBe(false);
  expect(currentAuth().ready()).toBe(false);
  expect(currentAuth().user()).toBeNull();
  expect(messages.slice(published)).toEqual([{ authChanging: true }]);
});

it("keeps the rendered identity when an identity refresh fails", async () => {
  responses.push(Response.json(account));
  mount();
  await waitFor(() => expect(currentAuth().ready()).toBe(true));
  const published = messages.length;
  const identity = currentAuth().identity();
  responses.push(new Error("network unavailable"));

  await expect(currentAuth().refreshIdentity()).rejects.toThrow("network unavailable");
  expect(currentAuth().ready()).toBe(true);
  expect(currentAuth().user()?.id).toBe("account-a");
  expect(currentAuth().identity()).toBe(identity);
  expect(messages).toHaveLength(published);
});

it("reconciles a probe with the rendered account", () => {
  const other = { ...account, id: "account-b", session_id: "session-b" };
  expect(reconcileIdentity(account, { user: { ...account, session_id: "session-b" } })).toEqual({
    status: "current",
    user: { ...account, session_id: "session-b" },
  });
  expect(reconcileIdentity(account, { user: other })).toEqual({ status: "changed", user: other });
  expect(reconcileIdentity(null, { user: account })).toEqual({ status: "changed", user: account });
  expect(reconcileIdentity(account, { error: new APIError(401, "UNAUTHORIZED") })).toEqual({
    status: "expired",
    user: null,
  });
  expect(reconcileIdentity(account, { error: new APIError(404, "NOT_FOUND") })).toEqual({
    status: "expired",
    user: null,
  });
  const offline = new Error("network unavailable");
  expect(reconcileIdentity(account, { error: offline })).toEqual({ status: "unavailable", user: null, error: offline });
});

it("keeps the recovered account when revoking the browser session fails", async () => {
  responses.push(Response.json(account));
  mount();
  await waitFor(() => expect(currentAuth().ready()).toBe(true));
  vi.spyOn(console, "error").mockImplementation(() => {});
  logoutResponses.push(Response.json({ error: { code: "BAD_REQUEST", message: "revoke failed" } }, { status: 400 }));
  responses.push(Response.json({ ...account, session_id: "session-b" }));

  await expect(currentAuth().logout()).rejects.toThrow("revoke failed");
  expect(currentAuth().ready()).toBe(true);
  expect(currentAuth().user()?.session_id).toBe("session-b");
  expect(document.querySelector('[role="alert"]')?.textContent).toBe("Sign-out failed. Try again.");
});

it("restores an unchanged session after a failed revocation", async () => {
  responses.push(Response.json(account));
  mount();
  await waitFor(() => expect(currentAuth().ready()).toBe(true));
  vi.spyOn(console, "error").mockImplementation(() => {});
  const published = messages.length;
  logoutResponses.push(Response.json({ error: { code: "BAD_REQUEST", message: "revoke failed" } }, { status: 400 }));
  responses.push(Response.json(account));

  await expect(currentAuth().logout()).rejects.toThrow("revoke failed");
  expect(currentAuth().ready()).toBe(true);
  expect(currentAuth().user()?.session_id).toBe("session-a");
  expect(messages.slice(published)).toEqual([{ authChanging: true }, { authChanged: true, nativeAccess: true }]);
});

it("adopts a different account after a failed revocation", async () => {
  responses.push(Response.json(account));
  mount();
  await waitFor(() => expect(currentAuth().ready()).toBe(true));
  vi.spyOn(console, "error").mockImplementation(() => {});
  logoutResponses.push(Response.json({ error: { code: "BAD_REQUEST", message: "revoke failed" } }, { status: 400 }));
  responses.push(Response.json({ ...account, id: "account-b", session_id: "session-b" }));

  await expect(currentAuth().logout()).rejects.toThrow("revoke failed");
  expect(currentAuth().ready()).toBe(true);
  expect(currentAuth().user()?.id).toBe("account-b");
  expect(currentAuth().user()?.session_id).toBe("session-b");
});

it("settles signed out when revocation recovery is unreachable", async () => {
  responses.push(Response.json(account));
  mount();
  await waitFor(() => expect(currentAuth().ready()).toBe(true));
  vi.spyOn(console, "error").mockImplementation(() => {});
  logoutResponses.push(Response.json({ error: { code: "BAD_REQUEST", message: "revoke failed" } }, { status: 400 }));
  responses.push(new Error("network unavailable"));

  await expect(currentAuth().logout()).rejects.toThrow("revoke failed");
  expect(currentAuth().ready()).toBe(true);
  expect(currentAuth().user()).toBeNull();
});

it("settles signed out when revocation fails and no cookie identity resolves", async () => {
  responses.push(Response.json(account));
  mount();
  await waitFor(() => expect(currentAuth().ready()).toBe(true));
  vi.spyOn(console, "error").mockImplementation(() => {});
  logoutResponses.push(Response.json({ error: { code: "BAD_REQUEST", message: "revoke failed" } }, { status: 400 }));
  responses.push(Response.json({ error: { code: "UNAUTHORIZED", message: "gone" } }, { status: 401 }));

  await expect(currentAuth().logout()).rejects.toThrow("revoke failed");
  expect(currentAuth().ready()).toBe(true);
  expect(currentAuth().user()).toBeNull();
  expect(document.querySelector('[role="alert"]')?.textContent).toBe("Sign-out failed. Try again.");
});
