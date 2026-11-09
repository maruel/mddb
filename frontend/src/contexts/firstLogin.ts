// Provisions the first workspace through server-side idempotent create requests.

import { OrgRoleAdmin, OrgRoleOwner, type UserResponse } from "@sdk/types.gen";
import type { Api } from "../useApi";

export async function provisionFirstWorkspace(
  user: UserResponse,
  api: Api,
  names: { organization: string; workspace: string },
  signal: AbortSignal,
): Promise<UserResponse> {
  let current = user;
  if (current.workspace_id) return current;

  if (!current.organizations?.length) {
    await api.createOrganization({ name: names.organization, ensure_first: true }, signal);
    current = await api.getMe(signal);
    if (current.id !== user.id) throw new Error("Account changed during first-login setup");
    if (current.workspace_id) return current;
  }

  const firstOrg = current.organizations?.[0];
  if (!firstOrg) throw new Error("First-login organization was not created");
  const workspaces = current.workspaces ?? [];
  let workspaceId = workspaces.find((ws) => ws.organization_id === firstOrg.organization_id)?.workspace_id;
  workspaceId ??= workspaces[0]?.workspace_id;
  if (!workspaceId) {
    if (firstOrg.role !== OrgRoleAdmin && firstOrg.role !== OrgRoleOwner) return current;
    const ws = await api
      .org(firstOrg.organization_id)
      .createWorkspace({ name: names.workspace, ensure_first: true }, signal);
    workspaceId = ws.id;
  }

  const switched = await api.switchWorkspace({ ws_id: workspaceId }, signal);
  if (!switched.user) throw new Error("First-login workspace switch returned no user");
  return switched.user;
}
