// E2E tests for removing an organization member with cascading workspace membership removal.

import { test, expect, registerUser, createClient } from './helpers';

test.describe('Remove Organization Member', () => {
  test('removing org member cascades to workspace memberships', async ({ page, request }) => {
    // Register owner (user A), onboard to create org+workspace.
    const userA = await registerUser(request, 'remove-member-a');
    await page.goto(`/?token=${userA.token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Get user A's org and workspace IDs.
    const clientA = createClient(request, userA.token);
    const meA = await clientA.auth.getMe();
    const orgId = meA.organization_id!;
    const wsId = meA.workspace_id!;
    expect(orgId).toBeTruthy();
    expect(wsId).toBeTruthy();

    // Register user B (no onboarding needed — they'll be added to A's org).
    const userB = await registerUser(request, 'remove-member-b');
    const clientB = createClient(request, userB.token);
    const meB = await clientB.auth.getMe();
    const userBId = meB.id;

    // User A adds user B to org as member and to workspace as editor.
    const orgApi = clientA.org(orgId);
    await orgApi.users.updateOrgMemberRole({ user_id: userBId, role: 'org:member' });
    const wsApi = clientA.ws(wsId);
    await wsApi.users.updateWSMemberRole({ user_id: userBId, role: 'ws:editor' });

    // Verify user B appears in org members list.
    const members = await orgApi.users.listUsers();
    expect(members.users.find((u) => u.id === userBId)).toBeTruthy();

    // Navigate to org settings members page.
    await page.goto(`/settings/org/${orgId}`);

    // Wait for the members table to load with user B's email.
    await expect(page.getByText(userB.email)).toBeVisible({ timeout: 10000 });

    // Accept the confirmation dialog when it appears.
    page.on('dialog', (dialog) => dialog.accept());

    // Click the Remove button in user B's row.
    const userBRow = page.locator('tr', { hasText: userB.email });
    const removeButton = userBRow.locator('button', { hasText: 'Remove' });
    await expect(removeButton).toBeVisible({ timeout: 5000 });
    await removeButton.click();

    // Wait for success message.
    await expect(page.locator('[class*="success"]')).toBeVisible({ timeout: 5000 });

    // Verify user B is no longer in the members table.
    await expect(page.getByText(userB.email)).not.toBeVisible({ timeout: 5000 });

    // Verify via API that user B lost both org and workspace memberships.
    const membersAfter = await orgApi.users.listUsers();
    expect(membersAfter.users.find((u) => u.id === userBId)).toBeUndefined();

    const meBAfter = await clientB.auth.getMe();
    expect(meBAfter.organizations?.find((o) => o.organization_id === orgId)).toBeUndefined();
    expect(meBAfter.workspaces?.find((w) => w.workspace_id === wsId)).toBeUndefined();
  });

  test('cannot remove yourself from organization', async ({ page, request }) => {
    const userA = await registerUser(request, 'remove-self');
    await page.goto(`/?token=${userA.token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const clientA = createClient(request, userA.token);
    const meA = await clientA.auth.getMe();
    const orgId = meA.organization_id!;
    expect(orgId).toBeTruthy();

    const orgApi = clientA.org(orgId);
    await expect(async () => {
      await orgApi.users.removeOrgMember({ user_id: meA.id });
    }).rejects.toThrow();
  });

  test('cannot remove the last owner', async ({ page, request }) => {
    const userA = await registerUser(request, 'remove-last-owner');
    await page.goto(`/?token=${userA.token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    const clientA = createClient(request, userA.token);
    const meA = await clientA.auth.getMe();
    const orgId = meA.organization_id!;
    expect(orgId).toBeTruthy();

    // Register user B and add as admin.
    const userB = await registerUser(request, 'remove-last-owner-b');
    const clientB = createClient(request, userB.token);
    const meB = await clientB.auth.getMe();

    const orgApi = clientA.org(orgId);
    await orgApi.users.updateOrgMemberRole({ user_id: meB.id, role: 'org:admin' });

    // User B tries to remove user A (the sole owner) — should fail.
    const orgApiB = clientB.org(orgId);
    await expect(async () => {
      await orgApiB.users.removeOrgMember({ user_id: meA.id });
    }).rejects.toThrow();
  });
});
