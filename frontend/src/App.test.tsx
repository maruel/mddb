// Tests the real App composition tree in jsdom: auth flow, sidebar node loading, workspace
// management, and onboarding. The network seam is stubbed through globalThis.fetch and
// localStorage is jsdom's real storage, so no child component needs mocking.
// Routing, node selection, settings navigation, and PWA install flows are covered by the
// Playwright e2e tests.

import { afterEach, beforeEach, describe, it } from "node:test";
import { expect, vi } from "@tests/expect";
import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import type { JSX } from "solid-js";
import App from "./App";
import { I18nProvider, useI18n } from "./i18n";
import type { UserResponse } from "@sdk/types.gen";
import { WSRoleViewer } from "@sdk/types.gen";

// --- Network seam -------------------------------------------------------------

const fetchCalls: Array<{ url: string; init?: RequestInit }> = [];

let fetchRoutes: Array<{
  match: string | RegExp;
  method?: string;
  respond: (url: string, init?: RequestInit | undefined) => unknown;
}> = [];

function jsonResponse(body: unknown, ok = true): Response {
  return {
    ok,
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
  return Promise.resolve(jsonResponse({}));
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
});

describe("App", () => {
  describe("Authentication", () => {
    it("uses the authenticated user's language instead of a previous account's stored language", async () => {
      localStorage.setItem("mddb_locale", "fr");
      localStorage.setItem("mddb_token", "existing-token");
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

    it("drops the old account when another tab replaces the token", async () => {
      localStorage.setItem("mddb_token", "existing-token");
      const otherUser = { ...mockUser, id: "user-2", name: "Other User" };
      api("GET", "/api/v1/auth/me", (_url, init) =>
        (init?.headers as Record<string, string> | undefined)?.Authorization === "Bearer new-token"
          ? otherUser
          : mockUser,
      );
      stubWorkspace();
      renderWithI18n(() => <App />);
      await waitFor(() => expect(screen.getByTestId("user-menu-button")).toBeTruthy());

      localStorage.setItem("mddb_token", "new-token");
      window.dispatchEvent(
        new window.StorageEvent("storage", {
          key: "mddb_token",
          oldValue: "existing-token",
          newValue: "new-token",
          storageArea: localStorage,
        }),
      );

      await waitFor(() => {
        const calls = fetchCalls.filter(
          ({ url, init }) =>
            url === "/api/v1/auth/me" &&
            (init?.headers as Record<string, string> | undefined)?.Authorization === "Bearer new-token",
        );
        expect(calls.length).toBeGreaterThan(0);
      });
      await waitFor(() => expect(screen.getByTestId("user-menu-button")).toHaveAttribute("title", "Other User"));
    });
    it("hands a validated bearer to Go Mode and clears it on logout", async () => {
      window.goModeHost = {};
      const postMessage = vi.fn();
      window.gomodeAuth = { postMessage };
      localStorage.setItem("mddb_token", "existing-token");
      let resolveUser!: (user: UserResponse) => void;
      const userResponse = new Promise<UserResponse>((resolve) => {
        resolveUser = resolve;
      });
      apiGet("/api/v1/auth/me", () => userResponse);
      stubWorkspace();
      api("POST", "/api/v1/auth/logout", () => ({}));

      renderWithI18n(() => <App />);
      expect(postMessage).not.toHaveBeenCalledWith(JSON.stringify({ bearerToken: "existing-token" }));

      resolveUser(mockUser);
      await waitFor(() => {
        expect(postMessage).toHaveBeenCalledWith(JSON.stringify({ bearerToken: "existing-token" }));
      });
      await waitFor(() => {
        expect(screen.getByTestId("user-menu-button")).toBeTruthy();
      });
      fireEvent.click(screen.getByTestId("user-menu-button"));
      fireEvent.click(screen.getByRole("menuitem", { name: "Logout" }));
      await waitFor(() => {
        expect(postMessage).toHaveBeenLastCalledWith(JSON.stringify({ bearerToken: null }));
      });
    });

    it("shows the login form when not logged in", async () => {
      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByRole("heading", { name: "Login to mddb" })).toBeTruthy();
      });
    });

    it("shows the main app when logged in via a localStorage token", async () => {
      localStorage.setItem("mddb_token", "existing-token");
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
      localStorage.setItem("mddb_token", "existing-token");
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

    it("stores the token and shows the main app after the login form submits", async () => {
      stubWorkspace();
      api("POST", "/api/v1/auth/login", () => ({ token: "test-token", user: mockUser }));

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByRole("heading", { name: "Login to mddb" })).toBeTruthy();
      });

      fireEvent.input(screen.getByLabelText("Email"), { target: { value: "test@example.com" } });
      fireEvent.input(screen.getByLabelText("Password"), { target: { value: "hunter2" } });
      fireEvent.click(screen.getByRole("button", { name: "Login" }));

      await waitFor(() => {
        expect(localStorage.getItem("mddb_token")).toBe("test-token");
      });
      await waitFor(() => {
        expect(screen.getByTestId("user-menu-button")).toBeTruthy();
      });
    });
  });

  describe("Node List", () => {
    it("loads and displays nodes in the sidebar", async () => {
      localStorage.setItem("mddb_token", "test-token");
      stubWorkspace();

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId("sidebar-node-node-1")).toBeTruthy();
      });
      expect(screen.getByText("Test Page")).toBeTruthy();
    });

    it("auto-selects the first node when nodes are loaded", async () => {
      localStorage.setItem("mddb_token", "test-token");
      stubWorkspace();

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId("sidebar-node-node-1")).toBeTruthy();
      });
    });
  });

  describe("Workspace Management", () => {
    it("ignores a node list that finishes after switching workspaces", async () => {
      localStorage.setItem("mddb_token", "test-token");
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
      api("POST", "/api/v1/auth/switch-workspace", () => ({ token: "new-token", user: secondUser }));
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
    it("auto-creates an organization when the user has no memberships", async () => {
      localStorage.setItem("mddb_token", "test-token");

      const userWithNoMemberships: UserResponse = {
        ...mockUser,
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
            role: "owner",
            created: 1704067200,
          },
        ],
        organization_id: "new-org-1",
        workspaces: [],
      };

      let getMeCallCount = 0;
      apiGet("/api/v1/auth/me", () => {
        getMeCallCount += 1;
        return getMeCallCount === 1 ? userWithNoMemberships : userAfterOrgCreation;
      });
      api("POST", "/api/v1/organizations", () => ({
        id: "new-org-1",
        name: "Test's Organization",
        settings: {},
        created: 1704067200,
        member_count: 1,
        workspace_count: 0,
      }));

      renderWithI18n(() => <App />);

      await waitFor(
        () => {
          const createOrgCalls = fetchCalls.filter(
            ({ url, init }) => url === "/api/v1/organizations" && init?.method === "POST",
          );
          expect(createOrgCalls.length).toBeGreaterThan(0);
        },
        { timeout: 3000 },
      );
    });

    it("shows the workspace switcher for users with multiple workspaces", async () => {
      localStorage.setItem("mddb_token", "test-token");

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
            role: "admin",
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
      localStorage.setItem("mddb_token", "test-token");
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
      localStorage.setItem("mddb_token", "test-token");
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
