// Verifies that a cookie account change in another tab replaces the visible account and workspace.

import { createClient, expect, getWorkspaceId, registerUser, test, gotoWithSession } from "./helpers";

test("another tab signing in replaces the current account and workspace", async ({ page, context, request }) => {
  const first = await registerUser(request, "storage-first");
  const second = await registerUser(request, "storage-second");
  const secondClient = createClient(request, second.token);
  const org = await secondClient.createOrganization({ name: "Second Account" });
  const workspace = await secondClient.org(org.id).createWorkspace({ name: "Second Workspace" });
  await secondClient.switchWorkspace({ ws_id: workspace.id });

  await gotoWithSession(page, first.token);
  await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });
  const firstWorkspace = await getWorkspaceId(page);
  await expect(page.getByTestId("user-menu-button")).toHaveAttribute("title", "storage-first Test User");

  const otherTab = await context.newPage();
  await otherTab.goto("/");
  await otherTab.getByTestId("user-menu-button").click();
  await otherTab.getByRole("menuitem", { name: "Logout", exact: true }).click();
  await otherTab.getByLabel("Email", { exact: true }).fill(second.email);
  await otherTab.getByLabel("Password", { exact: true }).fill("testpassword123");
  await otherTab.getByRole("button", { name: "Login", exact: true }).click();
  await expect(otherTab.locator("aside")).toBeVisible({ timeout: 15000 });
  const secondWorkspace = await getWorkspaceId(otherTab);
  expect(secondWorkspace).not.toBe(firstWorkspace);

  await expect(page.getByTestId("user-menu-button")).toHaveAttribute("title", "storage-second Test User");
  expect(secondWorkspace).toBe(workspace.id);
  await expect(page).toHaveURL(new RegExp(`/w/@${secondWorkspace}(?:[+/]|$)`));
  await otherTab.close();
});

test("two tabs provision one organization, workspace, and welcome page", async ({ page, context, request }) => {
  const { token } = await registerUser(request, "storage-first-login");
  const otherTab = await context.newPage();

  await Promise.all([gotoWithSession(page, token), gotoWithSession(otherTab, token)]);
  await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });
  await expect(otherTab.locator("aside")).toBeVisible({ timeout: 15000 });
  const workspaceId = await getWorkspaceId(page);
  expect(await getWorkspaceId(otherTab)).toBe(workspaceId);

  const client = createClient(request, token);
  await expect.poll(async () => (await client.getMe()).organizations?.length).toBe(1);
  await expect.poll(async () => (await client.getMe()).workspaces?.length).toBe(1);
  await expect.poll(async () => (await client.ws(workspaceId).listNodeChildren("0")).nodes?.length).toBe(1);
  await otherTab.close();
});
