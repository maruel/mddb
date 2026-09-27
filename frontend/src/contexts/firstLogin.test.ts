// Tests first-login provisioning decisions against a stateful API boundary.

import { describe, it } from "node:test";
import { expect } from "@tests/expect";
import { createApiClient, type FetchFn } from "@sdk/api.gen";
import {
  OrgRoleMember,
  OrgRoleOwner,
  WSRoleAdmin,
  type OrgMembershipResponse,
  type UserResponse,
  type WSMembershipResponse,
} from "@sdk/types.gen";
import { provisionFirstWorkspace } from "./firstLogin";

const names = { organization: "Test's Organization", workspace: "Test's Workspace" };

function setupApi(initial: { organization: boolean; workspace: boolean; member: boolean; otherWorkspace?: boolean }) {
  let organization = initial.organization;
  let workspace = initial.workspace;
  let activeWorkspaceId = initial.workspace ? "ws-1" : undefined;
  const requests: string[] = [];
  const currentUser = (): UserResponse => ({
    id: "user-1",
    email: "test@example.com",
    name: "Test User",
    settings: { theme: "light", language: "en" },
    created: 1,
    modified: 1,
    organization_id: activeWorkspaceId === "ws-2" ? "org-2" : organization ? "org-1" : undefined,
    workspace_id: activeWorkspaceId,
    organizations: organization
      ? [
          {
            id: "membership-1",
            user_id: "user-1",
            organization_id: "org-1",
            organization_name: names.organization,
            role: initial.member ? OrgRoleMember : OrgRoleOwner,
            created: 1,
          } satisfies OrgMembershipResponse,
          ...(initial.otherWorkspace
            ? [
                {
                  id: "membership-2",
                  user_id: "user-1",
                  organization_id: "org-2",
                  organization_name: "Other Organization",
                  role: OrgRoleMember,
                  created: 1,
                } satisfies OrgMembershipResponse,
              ]
            : []),
        ]
      : [],
    workspaces: [
      ...(workspace
        ? [
            {
              id: "workspace-membership-1",
              user_id: "user-1",
              workspace_id: "ws-1",
              workspace_name: names.workspace,
              organization_id: "org-1",
              role: WSRoleAdmin,
              settings: { notifications: true },
              created: 1,
            } satisfies WSMembershipResponse,
          ]
        : []),
      ...(initial.otherWorkspace
        ? [
            {
              id: "workspace-membership-2",
              user_id: "user-1",
              workspace_id: "ws-2",
              workspace_name: "Other Workspace",
              organization_id: "org-2",
              role: WSRoleAdmin,
              settings: { notifications: true },
              created: 1,
            } satisfies WSMembershipResponse,
          ]
        : []),
    ],
  });
  const fetchFn: FetchFn = async (url, init) => {
    const method = init?.method ?? "GET";
    requests.push(`${method} ${url}`);
    let body: unknown;
    if (url === "/api/v1/auth/me") body = currentUser();
    else if (url === "/api/v1/organizations" && method === "POST") {
      organization = true;
      body = { id: "org-1" };
    } else if (url === "/api/v1/organizations/org-1/workspaces" && method === "POST") {
      workspace = true;
      body = { id: "ws-1" };
    } else if (url === "/api/v1/auth/switch-workspace" && method === "POST") {
      activeWorkspaceId = (JSON.parse(String(init?.body)) as { ws_id: string }).ws_id;
      body = { token: "workspace-token", user: currentUser() };
    } else throw new Error(`Unexpected request: ${method} ${url}`);
    return { ok: true, json: async () => body } as Response;
  };
  return { api: createApiClient(fetchFn), requests };
}

describe("provisionFirstWorkspace", () => {
  it("creates one organization and workspace for a new account", async () => {
    const { api, requests } = setupApi({ organization: false, workspace: false, member: false });
    const result = await provisionFirstWorkspace("user-1", api, names);
    expect(result.user.workspace_id).toBe("ws-1");
    expect(result.token).toBe("workspace-token");
    expect(requests.filter((request) => request.startsWith("POST"))).toEqual([
      "POST /api/v1/organizations",
      "POST /api/v1/organizations/org-1/workspaces",
      "POST /api/v1/auth/switch-workspace",
    ]);
  });

  it("reuses the existing workspace without creating resources", async () => {
    const { api, requests } = setupApi({ organization: true, workspace: true, member: false });
    const result = await provisionFirstWorkspace("user-1", api, names);
    expect(result.user.workspace_id).toBe("ws-1");
    expect(result.token).toBeNull();
    expect(requests).toEqual(["GET /api/v1/auth/me"]);
  });

  it("leaves a member without a workspace for the manual first-workspace flow", async () => {
    const { api, requests } = setupApi({ organization: true, workspace: false, member: true });
    const result = await provisionFirstWorkspace("user-1", api, names);
    expect(result.user.workspace_id).toBeUndefined();
    expect(result.token).toBeNull();
    expect(requests).toEqual(["GET /api/v1/auth/me"]);
  });

  it("uses an accessible workspace in another organization when the first has none", async () => {
    const { api, requests } = setupApi({ organization: true, workspace: false, member: true, otherWorkspace: true });
    const result = await provisionFirstWorkspace("user-1", api, names);
    expect(result.user.workspace_id).toBe("ws-2");
    expect(result.token).toBe("workspace-token");
    expect(requests).toEqual(["GET /api/v1/auth/me", "POST /api/v1/auth/switch-workspace"]);
  });
});
