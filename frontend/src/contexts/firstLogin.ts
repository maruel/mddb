// Provisions the first workspace through server-side idempotent create requests.

import { OrgRoleAdmin, OrgRoleOwner, type UserResponse } from "@sdk/types.gen";
import type { Api } from "../useApi";

export interface FirstWorkspaceResult {
  user: UserResponse;
  token: string | null;
}

export async function provisionFirstWorkspace(
  userId: string,
  api: Api,
  names: { organization: string; workspace: string },
): Promise<FirstWorkspaceResult> {
  let current = await api.getMe();
  if (current.id !== userId) throw new Error("Account changed during first-login setup");
  if (current.workspace_id) return { user: current, token: null };

  if (!current.organizations?.length) {
    await api.createOrganization({ name: names.organization, ensure_first: true });
    current = await api.getMe();
  }

  const firstOrg = current.organizations?.[0];
  if (!firstOrg) throw new Error("First-login organization was not created");
  const workspaces = current.workspaces ?? [];
  let workspaceId = workspaces.find((ws) => ws.organization_id === firstOrg.organization_id)?.workspace_id;
  workspaceId ??= workspaces[0]?.workspace_id;
  if (!workspaceId) {
    if (firstOrg.role !== OrgRoleAdmin && firstOrg.role !== OrgRoleOwner) return { user: current, token: null };
    const ws = await api.org(firstOrg.organization_id).createWorkspace({ name: names.workspace, ensure_first: true });
    workspaceId = ws.id;
  }

  const switched = await api.switchWorkspace({ ws_id: workspaceId });
  if (!switched.user) throw new Error("First-login workspace switch returned no user");
  return { user: switched.user, token: switched.token };
}
