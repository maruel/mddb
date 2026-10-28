// Tests cookie-session ownership of native preparation across pending auth, session changes, and disposal.

import { afterEach, beforeEach, it } from "node:test";
import { cleanup, render, waitFor } from "@solidjs/testing-library";
import { expect, vi } from "@tests/expect";
import type { UserResponse } from "@sdk/types.gen";
import { WorkspaceDataProvider } from "../contexts/WorkspaceDataContext";
import { WorkspaceNavigationProvider } from "../contexts/WorkspaceNavigationContext";
import { AuthProvider, useAuth } from "../contexts/AuthContext";
import { Router, useNavigate } from "@solidjs/router";
import { HostModeProvider, useHostMode } from "@maruel/gomode/web/HostMode";
import { GoModeProvider, useGoMode } from "./GoMode";
import { I18nProvider } from "../i18n";

const account = { id: "account-a", session_id: "session-a", name: "Account A", settings: {} } as UserResponse;
const messages: Record<string, unknown>[] = [];
const readiness: Array<{ document: string; token: string; ready: boolean; tools: unknown[] }> = [];
let responses: Array<Promise<UserResponse>> = [];
let product: ReturnType<typeof useGoMode> | null = null;
let host: ReturnType<typeof useHostMode> | null = null;
let navigate: ReturnType<typeof useNavigate> | null = null;
let auth: ReturnType<typeof useAuth> | null = null;

function currentAuth() {
  if (!auth) throw new Error("Authentication provider did not mount");
  return auth;
}

function Probe() {
  auth = useAuth();
  product = useGoMode();
  host = useHostMode();
  navigate = useNavigate();
  return null;
}
function mount() {
  return render(() => (
    <I18nProvider initialLocale="en">
      <AuthProvider>
        <Router
          root={() => (
            <HostModeProvider>
              <WorkspaceDataProvider>
                <WorkspaceNavigationProvider>
                  <GoModeProvider>
                    <Probe />
                  </GoModeProvider>
                </WorkspaceNavigationProvider>
              </WorkspaceDataProvider>
            </HostModeProvider>
          )}
        />
      </AuthProvider>
    </I18nProvider>
  ));
}

beforeEach(() => {
  messages.length = 0;
  readiness.length = 0;
  responses = [];
  window.goModeHost = {};
  window.goModeFrontendDocument = Promise.resolve("originating-document");
  const bridge: NonNullable<Window["gomodeAuth"]> = {
    postMessage(message) {
      const value = JSON.parse(message) as Record<string, unknown>;
      messages.push(value);
      if (value.authChanged === true && value.nativeAccess === true) {
        bridge.onmessage?.({ data: JSON.stringify({ ...value, frontendToken: `identity-${messages.length}` }) });
      }
    },
  };
  window.gomodeAuth = bridge;
  window.gomodeFrontendReadiness = { postMessage: (message) => readiness.push(JSON.parse(message)) };
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    if (url.endsWith("/auth/me")) {
      const response = responses.shift();
      if (!response) throw new Error("No cookie identity response queued");
      return new Response(JSON.stringify(await response), { headers: { "Content-Type": "application/json" } });
    }
    if (url.endsWith("/nodes/0/children")) return Response.json({ nodes: [] });
    return new Response("{}", { headers: { "Content-Type": "application/json" } });
  });
});

afterEach(() => {
  cleanup();
  auth = null;
  delete window.goModeHost;
  delete window.goModeFrontendDocument;
  delete window.gomodeAuth;
  delete window.gomodeFrontendReadiness;
  vi.restoreAllMocks();
});

it("waits for settled cookie identity, explicitly prepares zero tools, and revokes on logout", async () => {
  const identity = Promise.withResolvers<UserResponse>();
  responses.push(identity.promise);
  mount();
  expect(currentAuth().ready()).toBe(false);
  expect(readiness).toEqual([]);
  identity.resolve(account);
  await waitFor(() => expect(readiness.at(-1)?.ready).toBe(true));
  expect(currentAuth().ready()).toBe(true);
  expect(readiness.at(-1)?.tools).toEqual([]);
  expect(messages.some((message) => "bearerToken" in message)).toBe(false);
  await currentAuth().logout();
  expect(readiness.at(-1)).toEqual(expect.objectContaining({ ready: false, tools: [] }));
  expect(messages.at(-1)).toEqual(expect.objectContaining({ authChanged: true, nativeAccess: false }));
});

it("keeps metadata refresh ready but replaces the lease for a new authoritative session", async () => {
  responses.push(Promise.resolve(account));
  mount();
  await waitFor(() => expect(readiness.at(-1)?.ready).toBe(true));
  const firstCount = readiness.length;
  const retiredReceive = window.gomodeAuth?.onmessage;
  const firstGrant = messages.find((message) => message.nativeAccess === true);
  responses.push(Promise.resolve({ ...account, name: "Updated name" }));
  await currentAuth().refreshUser();
  expect(readiness).toHaveLength(firstCount);
  const previousIdentity = currentAuth().identity();
  responses.push(Promise.resolve({ ...account, session_id: "session-b" }));
  await currentAuth().refreshUser();
  await waitFor(() => expect(readiness.at(-1)?.ready).toBe(true));
  expect(currentAuth().identity()).toBeGreaterThan(previousIdentity);
  expect(readiness.slice(firstCount).some((message) => !message.ready)).toBe(true);
  expect(readiness.at(-1)?.ready).toBe(true);
  const count = readiness.length;
  retiredReceive?.({ data: JSON.stringify({ ...firstGrant, frontendToken: "retired" }) });
  expect(readiness).toHaveLength(count);
});

it("cannot settle a disposed provider's pending cookie response over a replacement session", async () => {
  const retired = Promise.withResolvers<UserResponse>();
  responses.push(retired.promise);
  const first = mount();
  first.unmount();
  responses.push(Promise.resolve({ ...account, id: "account-b", session_id: "session-b" }));
  mount();
  await waitFor(() => expect(readiness.at(-1)?.ready).toBe(true));
  const count = readiness.length;
  retired.resolve(account);
  await retired.promise;
  await new Promise<void>((resolve) => setImmediate(resolve));
  expect(currentAuth().user()?.id).toBe("account-b");
  expect(readiness).toHaveLength(count);
});

it("captures all page bridges before cookie discovery awaits", async () => {
  const identity = Promise.withResolvers<UserResponse>();
  responses.push(identity.promise);
  mount();
  const replacementAuth = vi.fn();
  const replacementReadiness = vi.fn();
  window.goModeFrontendDocument = Promise.resolve("replacement-document");
  window.gomodeAuth = { postMessage: replacementAuth };
  window.gomodeFrontendReadiness = { postMessage: replacementReadiness };
  identity.resolve(account);
  await waitFor(() => expect(currentAuth().ready()).toBe(true));
  await Promise.resolve();
  expect(messages.every((message) => message.frontendDocument === "originating-document")).toBe(true);
  expect(replacementAuth).not.toHaveBeenCalled();
  expect(replacementReadiness).not.toHaveBeenCalled();
});

it("keeps preparation across settings routes, replaces workspace ownership, and exposes native voice state", async () => {
  window.history.replaceState(null, "", "/settings/user");
  responses.push(Promise.resolve({ ...account, workspace_id: "workspace-a" }));
  mount();
  await waitFor(() => expect(product?.preparation()?.ready()).toBe(true));
  const first = product!.preparation()!;
  navigate!("/settings/user");
  expect(product!.preparation()).toBe(first);
  navigate!("/w/@workspace-a/node");
  expect(product!.preparation()).toBe(first);
  let connected = false;
  window.goModeHost = { isVoiceConnected: () => connected };
  connected = true;
  window.dispatchEvent(new Event("gomodevoicechange"));
  expect(host!.nativeVoiceConnected()).toBe(true);
  expect(host!.browserVoiceEnabled()).toBe(false);
  navigate!("/settings/user");
  currentAuth().setUser({ ...account, workspace_id: "workspace-b" });
  expect(first.ready()).toBe(false);
  expect(product!.preparation()).not.toBe(first);
  await waitFor(() => expect(product!.preparation()?.ready()).toBe(true));
  cleanup();
  expect(product!.preparation()?.ready()).toBe(false);
});
