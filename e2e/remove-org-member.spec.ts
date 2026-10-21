// E2E tests for removing an organization member with cascading workspace membership removal.

import { test, expect, registerUser, createClient } from "./helpers";

test.describe("Remove Organization Member", () => {
  test("responsive member rows allow role changes and cascading removal", async ({ page, request }, testInfo) => {
    // Register owner (user A), onboard to create org+workspace.
    const userA = await registerUser(request, "remove-member-a");
    await page.goto(`/?token=${userA.token}`);
    await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });

    // Get user A's org and workspace IDs.
    const clientA = createClient(request, userA.token);
    const meA = await clientA.getMe();
    const orgId = meA.organization_id!;
    const wsId = meA.workspace_id!;
    expect(orgId).toBeTruthy();
    expect(wsId).toBeTruthy();

    // Register user B (no onboarding needed — they'll be added to A's org).
    const userB = await registerUser(request, "Averylongmembernamewithoutspacesforresponsivememberlayout");
    const clientB = createClient(request, userB.token);
    const meB = await clientB.getMe();
    const userBId = meB.id;

    // User A adds user B to org as member and to workspace as editor.
    const orgApi = clientA.org(orgId);
    await orgApi.updateOrgMemberRole({ user_id: userBId, role: "org:member" });
    const wsApi = clientA.ws(wsId);
    await wsApi.updateWSMemberRole({ user_id: userBId, role: "ws:editor" });

    // Verify user B appears in org members list.
    const members = await orgApi.listUsers();
    expect(members.users.find((u) => u.id === userBId)).toBeTruthy();

    // Navigate to org settings members page.
    await page.goto(`/settings/org/${orgId}`);

    // Wait for the members table to load with user B's email.
    await expect(page.getByText(userB.email)).toBeVisible({ timeout: 10000 });

    const userBRow = page.getByRole("row").filter({ hasText: userB.email });
    const table = page.getByRole("table").filter({ has: userBRow });
    for (const width of [1280, 390, 195]) {
      await page.setViewportSize({ width, height: 900 });
      await userBRow.scrollIntoViewIfNeeded();
      const viewport = await page.evaluate(() => window.innerWidth);
      for (const element of [table, userBRow, ...(await userBRow.getByRole("cell").all())]) {
        const box = await element.boundingBox();
        if (box === null) throw new Error("Member row is not visible");
        expect(box.x).toBeGreaterThanOrEqual(0);
        expect(box.x + box.width).toBeLessThanOrEqual(viewport);
        expect(await element.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
      }
      await page.screenshot({ path: testInfo.outputPath(`member-rows-${width}.png`) });
    }
    await userBRow.getByRole("combobox").selectOption("org:admin");
    await expect
      .poll(async () => (await clientB.getMe()).organizations?.find((m) => m.organization_id === orgId)?.role)
      .toBe("org:admin");
    await page.setViewportSize({ width: 390, height: 900 });
    page.on("dialog", (dialog) => dialog.accept());
    const removeButton = userBRow.getByRole("button", { name: "Remove", exact: true });
    await expect(removeButton).toBeVisible({ timeout: 5000 });
    await removeButton.click();

    // Wait for success message.
    await expect(page.locator('[class*="success"]')).toBeVisible({ timeout: 5000 });

    // Verify user B is no longer in the members table.
    await expect(page.getByText(userB.email)).not.toBeVisible({ timeout: 5000 });

    // Verify via API that user B lost both org and workspace memberships.
    const membersAfter = await orgApi.listUsers();
    expect(membersAfter.users.find((u) => u.id === userBId)).toBeUndefined();

    const meBAfter = await clientB.getMe();
    expect(meBAfter.organizations?.find((o) => o.organization_id === orgId)).toBeUndefined();
    expect(meBAfter.workspaces?.find((w) => w.workspace_id === wsId)).toBeUndefined();
  });

  test("cannot remove yourself from organization", async ({ page, request }) => {
    const userA = await registerUser(request, "remove-self");
    await page.goto(`/?token=${userA.token}`);
    await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });

    const clientA = createClient(request, userA.token);
    const meA = await clientA.getMe();
    const orgId = meA.organization_id!;
    expect(orgId).toBeTruthy();

    const orgApi = clientA.org(orgId);
    await expect(async () => {
      await orgApi.removeOrgMember({ user_id: meA.id });
    }).rejects.toThrow();
  });

  test("cannot remove the last owner", async ({ page, request }) => {
    const userA = await registerUser(request, "remove-last-owner");
    await page.goto(`/?token=${userA.token}`);
    await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });

    const clientA = createClient(request, userA.token);
    const meA = await clientA.getMe();
    const orgId = meA.organization_id!;
    expect(orgId).toBeTruthy();

    // Register user B and add as admin.
    const userB = await registerUser(request, "remove-last-owner-b");
    const clientB = createClient(request, userB.token);
    const meB = await clientB.getMe();

    const orgApi = clientA.org(orgId);
    await orgApi.updateOrgMemberRole({ user_id: meB.id, role: "org:admin" });

    // User B tries to remove user A (the sole owner) — should fail.
    const orgApiB = clientB.org(orgId);
    await expect(async () => {
      await orgApiB.removeOrgMember({ user_id: meA.id });
    }).rejects.toThrow();
  });
});
