// Tests in-place cookie identity refresh and the reload fallbacks that own a fresh document.

import { afterEach, beforeEach, it } from "node:test";
import { cleanup, render, waitFor } from "@solidjs/testing-library";
import { expect, vi } from "@tests/expect";
import type { UserResponse } from "@sdk/types.gen";
import { I18nProvider } from "../i18n";
import { AuthProvider, useAuth } from "./AuthContext";

const account = {
  id: "account-a",
  email: "account-a@example.com",
  name: "Account A",
  session_id: "session-a",
  settings: {},
} as UserResponse;
const messages: Record<string, unknown>[] = [];
let responses: Array<Response | Error> = [];
let auth: ReturnType<typeof useAuth> | null = null;

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
    expect(url).toBe("/api/v1/auth/me");
    const response = responses.shift();
    if (response instanceof Error) throw response;
    if (!response) throw new Error("No cookie identity response queued");
    return response;
  });
});

afterEach(() => {
  cleanup();
  auth = null;
  delete window.goModeFrontendDocument;
  delete window.gomodeAuth;
  vi.restoreAllMocks();
});

it("refreshes a same-account session in place", async () => {
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
