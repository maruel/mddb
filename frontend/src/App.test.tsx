// Tests the real App composition tree in jsdom: auth flow, sidebar node loading, workspace
// management, and onboarding. The network seam is stubbed through globalThis.fetch and
// localStorage is jsdom's real storage, so no child component needs mocking.
// Browser-specific navigation and PWA installation remain covered by Playwright.

import { afterEach, beforeEach, describe, it } from "node:test";
import { expect, vi } from "@tests/expect";
import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import type { JSX } from "solid-js";
import { Router, Route } from "@solidjs/router";
import App from "./App";
import { I18nProvider, useI18n } from "./i18n";
import { AuthProvider, useAuth, type AuthContextValue } from "./contexts/AuthContext";
import { WorkspaceProvider, useWorkspace } from "./contexts/WorkspaceContext";
import { EventSourceProvider } from "./contexts/EventSourceContext";
import { NotificationProvider, useNotifications } from "./contexts/NotificationContext";
import { createApi } from "./useApi";
import type { UserResponse } from "@sdk/types.gen";
import { WSRoleViewer } from "@sdk/types.gen";

// --- Network seam -------------------------------------------------------------

const fetchCalls: Array<{ url: string; init?: RequestInit }> = [];
const originalServiceWorker = Object.getOwnPropertyDescriptor(navigator, "serviceWorker");

let fetchRoutes: Array<{
  match: string | RegExp;
  method?: string;
  respond: (url: string, init?: RequestInit | undefined) => unknown;
}> = [];

function jsonResponse(body: unknown, ok = true): Response {
  return {
    ok,
    status: ok ? 200 : 401,
    headers: new Headers({ "content-type": "application/json" }),
    json: () => Promise.resolve(body),
  } as unknown as Response;
}

globalThis.fetch = ((request: RequestInfo | URL, init?: RequestInit) => {
  const url = typeof request === "string" ? request : request instanceof URL ? request.href : request.url;
  fetchCalls.push({ url, init });
  for (const route of fetchRoutes) {
    if (route.method && (init?.method ?? "GET") !== route.method) continue;
    if (typeof route.match === "string" ? url === route.match : route.match.test(url)) {
      return Promise.resolve(jsonResponse(route.respond(url, init)));
    }
  }
  return Promise.resolve(
    jsonResponse({ error: { code: "UNAUTHORIZED", message: "Sign in required" } }, !url.endsWith("/auth/me")),
  );
}) as typeof fetch;

function api(method: string, match: string | RegExp, respond: (url: string, init?: RequestInit) => unknown): void {
  fetchRoutes.push({ method, match, respond });
}

function apiGet(match: string | RegExp, respond: (url: string) => unknown): void {
  api("GET", match, (url) => respond(url));
}

// --- Fixtures -----------------------------------------------------------------

const mockUser: UserResponse = {
  id: "user-1",
  email: "test@example.com",
  name: "Test User",
  organization_id: "org-1",
  org_role: "org:member",
  workspace_id: "ws-1",
  workspace_name: "Test Workspace",
  workspace_role: WSRoleViewer,
  organizations: [
    {
      id: "mem-1",
      user_id: "user-1",
      organization_id: "org-1",
      organization_name: "Test Org",
      role: "org:member",
      created: 1704067200,
    },
  ],
  workspaces: [
    {
      id: "wsmem-1",
      user_id: "user-1",
      workspace_id: "ws-1",
      workspace_name: "Test Workspace",
      organization_id: "org-1",
      role: WSRoleViewer,
      settings: { notifications: true },
      created: 1704067200,
    },
  ],
  settings: { theme: "light", language: "en" },
  created: 1704067200,
  modified: 1704067200,
};

const mockNodes = [
  {
    id: "node-1",
    title: "Test Page",
    content: "# Hello World",
    created: 1704067200,
    modified: 1704067200,
    has_page: true,
    has_table: false,
  },
  {
    id: "node-2",
    title: "Test Table",
    properties: [
      { name: "Name", type: "text", required: true },
      {
        name: "Status",
        type: "select",
        options: [
          { id: "opt-1", name: "Todo" },
          { id: "opt-2", name: "Done" },
        ],
      },
    ],
    created: 1704067200,
    modified: 1704067200,
    has_page: false,
    has_table: true,
  },
];

// Routes serving a logged-in workspace: current user, the root node, and its children.
function stubWorkspace(user: UserResponse = mockUser, nodes = mockNodes): void {
  apiGet("/api/v1/auth/me", () => user);
  apiGet(/\/nodes\/0$/, () => ({
    id: "0",
    title: "Root",
    created: 1704067200,
    modified: 1704067200,
    has_page: true,
    has_table: false,
    children: [],
  }));
  apiGet(/\/nodes\/0\/children$/, () => ({ nodes }));
}

// --- Harness ------------------------------------------------------------------

function renderWithI18n(component: () => JSX.Element) {
  return render(() => <I18nProvider>{component()}</I18nProvider>);
}

function LocaleStatus() {
  const { locale } = useI18n();
  return <output data-testid="locale">{locale()}</output>;
}

beforeEach(() => {
  cleanup();
  vi.clearAllMocks();
  fetchCalls.length = 0;
  fetchRoutes = [];
  localStorage.clear();
  delete window.goModeHost;
  delete window.gomodeAuth;
  // Reset URL to root for each test.
  window.history.replaceState(null, "", "/");
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  if (originalServiceWorker) Object.defineProperty(navigator, "serviceWorker", originalServiceWorker);
  else Reflect.deleteProperty(navigator, "serviceWorker");
});

describe("App", () => {
  describe("Authentication", () => {
    it("does not subscribe replacement-session push after a delayed originating VAPID response", async () => {
      let context!: ReturnType<typeof useNotifications>;
      function Probe() {
        context = useNotifications();
        return null;
      }
      const subscribe = vi.fn(async () => ({
        toJSON: () => ({ endpoint: "https://push.example.test/sub", keys: { p256dh: "key", auth: "auth" } }),
      }));
      Object.defineProperty(navigator, "serviceWorker", {
        configurable: true,
        value: { ready: Promise.resolve({ pushManager: { getSubscription: async () => null, subscribe } }) },
      });
      vi.stubGlobal("Notification", { requestPermission: async () => "granted" });
      let resolveVapid!: (value: unknown) => void;
      const vapid = new Promise<unknown>((resolve) => {
        resolveVapid = resolve;
      });
      apiGet("/api/v1/notifications/vapid-key", () => vapid);
      let client = createApi(undefined, "session-A");
      render(() => (
        <NotificationProvider api={() => client}>
          <Probe />
        </NotificationProvider>
      ));
      const pending = context.enablePush();
      await waitFor(() => expect(fetchCalls.some(({ url }) => url.endsWith("/vapid-key"))).toBe(true));
      client = createApi(undefined, "session-B");
      resolveVapid({ public_key: "AQ" });
      expect(await pending).toBe(false);
      expect(subscribe).not.toHaveBeenCalled();
      expect(fetchCalls.filter(({ url }) => url.endsWith("/notifications/subscribe"))).toHaveLength(0);
    });
    it("settles failed logout and failed recovery with a persistent visible error", async () => {
      window.goModeHost = {};
      const postMessage = vi.fn();
      window.gomodeAuth = { postMessage };
      let meCalls = 0;
      apiGet("/api/v1/auth/me", () => {
        if (meCalls++ === 0) return mockUser;
        throw new Error("recovery offline");
      });
      api("POST", "/api/v1/auth/browser/logout", () => {
        throw new Error("logout offline");
      });
      stubWorkspace();
      renderWithI18n(() => <App />);
      await waitFor(() => expect(screen.getByTestId("user-menu-button")).toBeTruthy());
      fireEvent.click(screen.getByTestId("user-menu-button"));
      fireEvent.click(screen.getByRole("menuitem", { name: "Logout" }));
      await waitFor(() => expect(screen.getByRole("heading", { name: "Login to mddb" })).toBeTruthy());
      expect(screen.getByRole("alert")).toHaveTextContent("Sign-out failed");
      expect(postMessage).toHaveBeenLastCalledWith(JSON.stringify({ authChanged: true, nativeAccess: false }));
    });

    for (const operation of ["switch", "organization", "workspace"] as const) {
      it(`discards delayed ${operation} continuations after session replacement`, async () => {
        let auth!: AuthContextValue;
        let workspace!: ReturnType<typeof useWorkspace>;
        function Probe() {
          auth = useAuth();
          workspace = useWorkspace();
          return null;
        }
        const initial = { ...mockUser, session_id: "session-A" };
        apiGet("/api/v1/auth/me", () => initial);
        stubWorkspace();
        let resolveOperation!: (value: unknown) => void;
        const delayed = new Promise<unknown>((resolve) => {
          resolveOperation = resolve;
        });
        const path =
          operation === "switch"
            ? "/api/v1/auth/switch-workspace"
            : operation === "organization"
              ? "/api/v1/organizations"
              : "/api/v1/organizations/org-1/workspaces";
        api("POST", path, () => delayed);
        renderWithI18n(() => (
          <Router
            root={(props) => (
              <AuthProvider>
                <EventSourceProvider>
                  <WorkspaceProvider>{props.children}</WorkspaceProvider>
                </EventSourceProvider>
              </AuthProvider>
            )}
          >
            <Route path="*" component={Probe} />
          </Router>
        ));
        await waitFor(() => expect(auth.ready()).toBe(true));
        const pending =
          operation === "switch"
            ? workspace.switchWorkspace("ws-old")
            : operation === "organization"
              ? workspace.createOrganization({ name: "Old org" })
              : workspace.createWorkspace({ name: "Old workspace" });
        const count = fetchCalls.length;
        auth.login({ ...mockUser, id: "replacement", session_id: "session-B" });
        resolveOperation(
          operation === "switch" ? { user: { ...initial, workspace_id: "ws-old" } } : { id: "created-old" },
        );
        await pending;
        expect(auth.user()?.id).toBe("replacement");
        expect(auth.user()?.session_id).toBe("session-B");
        expect(
          fetchCalls
            .slice(count)
            .filter(({ url }) => url === "/api/v1/auth/me" || url === "/api/v1/auth/switch-workspace"),
        ).toHaveLength(0);
        expect(
          new Headers(fetchCalls.find(({ url, init }) => url === path && init?.method === "POST")?.init?.headers).get(
            "X-MDDB-Session",
          ),
        ).toBe("session-A");
      });
    }
    it("uses the authenticated user's language instead of a previous account's stored language", async () => {
      localStorage.setItem("mddb_locale", "fr");
      stubWorkspace();

      render(() => (
        <I18nProvider initialLocale="fr">
          <App />
          <LocaleStatus />
        </I18nProvider>
      ));

      await waitFor(() => expect(screen.getByTestId("user-menu-button")).toBeTruthy());
      expect(screen.getByTestId("locale")).toHaveTextContent("en");
    });

    it("drops the old account when another tab replaces the cookie session", async () => {
      window.history.replaceState(null, "", "/w/@ws-1+test-workspace/@node-1+test-page");
      const otherUser = {
        ...mockUser,
        id: "user-2",
        name: "Other User",
        workspace_id: "ws-2",
        workspace_name: "Other Workspace",
      };
      let currentUser = mockUser;
      api("GET", "/api/v1/auth/me", () => currentUser);
      stubWorkspace();
      renderWithI18n(() => <App />);
      await waitFor(() => expect(screen.getByTestId("user-menu-button")).toBeTruthy());

      currentUser = otherUser;
      localStorage.setItem("mddb_auth_changed", "new-event");
      window.dispatchEvent(
        new window.StorageEvent("storage", {
          key: "mddb_auth_changed",
          oldValue: "existing-token",
          newValue: "new-token",
          storageArea: localStorage,
        }),
      );

      await waitFor(() => {
        const calls = fetchCalls.filter(({ url }) => url === "/api/v1/auth/me");
        expect(calls.length).toBeGreaterThan(0);
      });
      await waitFor(() => expect(screen.getByTestId("user-menu-button")).toHaveAttribute("title", "Other User"));
      await waitFor(() => expect(window.location.pathname).toMatch(/^\/w\/@ws-2(?:[+/]|$)/));
    });

    it("discards stale account lookups and pins operations to their original session", async () => {
      let auth!: AuthContextValue;
      function Probe() {
        auth = useAuth();
        return null;
      }
      let resolveOld!: (value: UserResponse) => void;
      const oldLookup = new Promise<UserResponse>((resolve) => {
        resolveOld = resolve;
      });
      let calls = 0;
      let replacement = { ...mockUser, session_id: "session-2" };
      apiGet("/api/v1/auth/me", () => (calls++ === 0 ? oldLookup : replacement));
      renderWithI18n(() => (
        <AuthProvider>
          <Probe />
        </AuthProvider>
      ));
      window.dispatchEvent(
        new window.StorageEvent("storage", {
          key: "mddb_auth_changed",
          newValue: "nonsecret-event",
          storageArea: localStorage,
        }),
      );
      await waitFor(() => expect(auth.user()?.session_id).toBe("session-2"));
      const originalApi = auth.api();
      const epoch = auth.identity();
      resolveOld({ ...mockUser, id: "stale-account", session_id: "session-1" });
      await new Promise<void>((resolve) => setTimeout(resolve, 0));
      expect(auth.user()?.id).toBe(mockUser.id);
      expect(auth.user()?.session_id).toBe("session-2");

      // Same account, different session: replacement sessions still invalidate
      // outstanding work. The old client must never acquire the new pin.
      replacement = { ...replacement, session_id: "session-3" };
      await auth.refreshUser();
      expect(auth.identity()).toBeGreaterThan(epoch);
      await originalApi.getMe();
      expect(new Headers(fetchCalls.at(-1)?.init?.headers).get("X-MDDB-Session")).toBe("session-2");
      await auth.api().getMe();
      expect(new Headers(fetchCalls.at(-1)?.init?.headers).get("X-MDDB-Session")).toBe("session-3");
    });

    it("signals settled cookie authentication to Go Mode and pauses it on logout", async () => {
      window.goModeHost = {};
      const postMessage = vi.fn();
      window.gomodeAuth = { postMessage };
      let resolveUser!: (user: UserResponse) => void;
      const userResponse = new Promise<UserResponse>((resolve) => {
        resolveUser = resolve;
      });
      apiGet("/api/v1/auth/me", () => userResponse);
      stubWorkspace();
      api("POST", "/api/v1/auth/browser/logout", () => ({}));

      renderWithI18n(() => <App />);
      expect(postMessage).not.toHaveBeenCalledWith(JSON.stringify({ authChanged: true, nativeAccess: true }));

      resolveUser(mockUser);
      await waitFor(() => {
        expect(postMessage).toHaveBeenCalledWith(JSON.stringify({ authChanged: true, nativeAccess: true }));
      });
      await waitFor(() => {
        expect(screen.getByTestId("user-menu-button")).toBeTruthy();
      });
      fireEvent.click(screen.getByTestId("user-menu-button"));
      fireEvent.click(screen.getByRole("menuitem", { name: "Logout" }));
      await waitFor(() => {
        expect(postMessage).toHaveBeenLastCalledWith(JSON.stringify({ authChanged: true, nativeAccess: false }));
      });
    });

    it("shows the login form when not logged in", async () => {
      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByRole("heading", { name: "Login to mddb" })).toBeTruthy();
      });
    });

    it("shows the main app when logged in through a cookie-authenticated account lookup", async () => {
      stubWorkspace();

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId("user-menu-button")).toBeTruthy();
      });
    });

    it("shows the browser voice overlay only when the host advertises a gateway", async () => {
      vi.stubGlobal(
        "ResizeObserver",
        class {
          observe() {}
          unobserve() {}
          disconnect() {}
        },
      );
      stubWorkspace();
      apiGet("/.well-known/gomode.json", () => ({
        service: "mddb",
        webShell: {
          toolGroups: [{ name: "workspace", endpoint: "/api/v1/gomode/mcp" }],
          voiceGateway: { url: "/", authRequired: true },
        },
      }));

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId("voice-overlay")).toBeTruthy();
      });
      expect(screen.getByRole("button", { name: "Connect voice assistant" })).toBeTruthy();
    });

    it("keeps credentials out of storage and shows the main app after the login form submits", async () => {
      apiGet("/api/v1/auth/me", () => {
        throw new Error("not signed in");
      });
      stubWorkspace();
      api("POST", "/api/v1/auth/browser/login", () => mockUser);

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByRole("heading", { name: "Login to mddb" })).toBeTruthy();
      });

      fireEvent.input(screen.getByLabelText("Email"), { target: { value: "test@example.com" } });
      fireEvent.input(screen.getByLabelText("Password"), { target: { value: "hunter2" } });
      fireEvent.click(screen.getByRole("button", { name: "Login" }));

      await waitFor(() => {
        expect(localStorage.getItem("mddb_token")).toBeNull();
      });
      await waitFor(() => {
        expect(screen.getByTestId("user-menu-button")).toBeTruthy();
      });
    });
  });

  describe("Node List", () => {
    it("loads and displays nodes in the sidebar", async () => {
      stubWorkspace();

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId("sidebar-node-node-1")).toBeTruthy();
      });
      expect(screen.getByText("Test Page")).toBeTruthy();
    });

    it("auto-selects the first node when nodes are loaded", async () => {
      stubWorkspace();

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId("sidebar-node-node-1")).toBeTruthy();
      });
    });
  });

  describe("Workspace Management", () => {
    it("ignores a node list that finishes after switching workspaces", async () => {
      const firstMembership = mockUser.workspaces?.[0];
      if (!firstMembership) throw new Error("Missing workspace fixture");
      const otherWorkspace = {
        ...firstMembership,
        id: "wsmem-2",
        workspace_id: "ws-2",
        workspace_name: "Other Workspace",
      };
      const firstUser = { ...mockUser, workspaces: [...(mockUser.workspaces ?? []), otherWorkspace] };
      const secondUser = { ...firstUser, workspace_id: "ws-2", workspace_name: "Other Workspace" };
      let resolveOldNodes!: (nodes: { nodes: typeof mockNodes }) => void;
      const oldNodes = new Promise<{ nodes: typeof mockNodes }>((resolve) => {
        resolveOldNodes = resolve;
      });
      apiGet(/\/workspaces\/ws-1\/nodes\/0\/children$/, () => oldNodes);
      apiGet(/\/workspaces\/ws-2\/nodes\/0\/children$/, () => ({
        nodes: [{ ...mockNodes[0]!, id: "other-node", title: "Other Workspace Page" }],
      }));
      apiGet(/\/workspaces\/ws-2\/nodes\/other-node$/, () => ({
        ...mockNodes[0]!,
        id: "other-node",
        title: "Other Workspace Page",
      }));
      api("POST", "/api/v1/auth/switch-workspace", () => ({ user: secondUser }));
      stubWorkspace(firstUser);

      renderWithI18n(() => <App />);
      await waitFor(() => expect(screen.getByRole("button", { name: /Other workspaces/i })).toBeTruthy());
      fireEvent.click(screen.getByRole("button", { name: /Other workspaces/i }));
      fireEvent.click(screen.getByRole("button", { name: "Other Workspace" }));
      await waitFor(() => expect(screen.getByTestId("sidebar-node-other-node")).toBeTruthy());

      resolveOldNodes({ nodes: mockNodes });
      await new Promise((resolve) => setTimeout(resolve, 0));
      expect(screen.queryByTestId("sidebar-node-node-1")).toBeNull();
      expect(screen.getByTestId("sidebar-node-other-node")).toBeTruthy();
    });
    it("auto-provisions an organization and workspace when the user has no memberships", async () => {
      const userWithNoMemberships: UserResponse = {
        ...mockUser,
        workspace_id: undefined,
        workspace_name: undefined,
        organizations: [],
        workspaces: [],
      };

      // After org creation, the refreshed user has an org (named after the user's first name).
      const userAfterOrgCreation: UserResponse = {
        ...mockUser,
        organizations: [
          {
            id: "new-membership-1",
            user_id: "user-1",
            organization_id: "new-org-1",
            organization_name: "Test's Organization",
            role: "org:owner",
            created: 1704067200,
          },
        ],
        organization_id: "new-org-1",
        workspace_id: undefined,
        workspace_name: undefined,
        workspaces: [],
      };
      const userAfterWorkspaceCreation: UserResponse = {
        ...userAfterOrgCreation,
        workspace_id: "new-ws-1",
        workspace_name: "Test's Workspace",
        workspaces: [
          {
            ...mockUser.workspaces![0]!,
            workspace_id: "new-ws-1",
            workspace_name: "Test's Workspace",
            organization_id: "new-org-1",
          },
        ],
      };

      let orgCreated = false;
      let workspaceCreated = false;
      apiGet("/api/v1/auth/me", () =>
        workspaceCreated ? userAfterWorkspaceCreation : orgCreated ? userAfterOrgCreation : userWithNoMemberships,
      );
      api("POST", "/api/v1/organizations", () => {
        orgCreated = true;
        return { id: "new-org-1", name: "Test's Organization" };
      });
      api("POST", "/api/v1/organizations/new-org-1/workspaces", () => {
        workspaceCreated = true;
        return { id: "new-ws-1", name: "Test's Workspace" };
      });
      api("POST", "/api/v1/auth/switch-workspace", () => ({ user: userAfterWorkspaceCreation }));
      apiGet(/\/nodes\/0\/children$/, () => ({ nodes: mockNodes }));

      renderWithI18n(() => <App />);

      await waitFor(
        () => {
          const createOrgCalls = fetchCalls.filter(
            ({ url, init }) => url === "/api/v1/organizations" && init?.method === "POST",
          );
          expect(createOrgCalls).toHaveLength(1);
          expect(fetchCalls.filter(({ url }) => url === "/api/v1/organizations/new-org-1/workspaces")).toHaveLength(1);
        },
        { timeout: 3000 },
      );
    });

    it("shows the workspace switcher for users with multiple workspaces", async () => {
      const userWithMultipleOrgs: UserResponse = {
        ...mockUser,
        organizations: [
          {
            id: "mem-1",
            user_id: "user-1",
            organization_id: "org-1",
            organization_name: "Org 1",
            role: "org:member",
            created: 1704067200,
          },
          {
            id: "mem-2",
            user_id: "user-1",
            organization_id: "org-2",
            organization_name: "Org 2",
            role: "org:admin",
            created: 1704067200,
          },
        ],
      };
      stubWorkspace(userWithMultipleOrgs);

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId("create-workspace-button")).toBeTruthy();
      });
    });

    it("opens the create workspace modal from the sidebar", async () => {
      stubWorkspace();

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId("create-workspace-button")).toBeTruthy();
      });

      fireEvent.click(screen.getByTestId("create-workspace-button"));

      await waitFor(() => {
        expect(screen.getByRole("heading", { name: "Create Workspace" })).toBeTruthy();
      });
    });
  });

  describe("Onboarding", () => {
    it("does not show onboarding on initial load", async () => {
      stubWorkspace();

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId("user-menu-button")).toBeTruthy();
      });

      // Onboarding is only shown after organization creation, not on initial load.
      expect(screen.queryByRole("heading", { name: "Welcome to mddb!" })).toBeNull();
    });
  });

  describe("PWA Banner", () => {
    it("defers installation during sign-in and retains the offer on ordinary routes", async () => {
      renderWithI18n(() => <App />);
      await waitFor(() => expect(screen.getByRole("heading", { name: "Login to mddb" })).toBeTruthy());
      window.dispatchEvent(
        Object.assign(new Event("beforeinstallprompt"), {
          prompt: async () => undefined,
          userChoice: Promise.resolve({ outcome: "dismissed" }),
        }),
      );
      expect(screen.queryByText("Install mddb")).toBeNull();
      fireEvent.click(screen.getByRole("link", { name: "Privacy Policy" }));
      await waitFor(() => expect(screen.getByText("Install mddb")).toBeTruthy());
      expect(localStorage.getItem("mddb_pwa_dismissed")).toBeNull();
    });
    it("hides the install banner until the browser offers installation", async () => {
      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByRole("heading", { name: "Login to mddb" })).toBeTruthy();
      });

      // jsdom never fires beforeinstallprompt, so the banner must stay hidden.
      expect(screen.queryByText("Install mddb")).toBeNull();
    });
  });
});
