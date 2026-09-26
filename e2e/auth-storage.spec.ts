// Verifies that a token change in another tab replaces the visible account and workspace.

import { expect, getWorkspaceId, registerUser, test } from "./helpers";

test("another tab signing in replaces the current account and workspace", async ({ page, context, request }) => {
  const first = await registerUser(request, "storage-first");
  const second = await registerUser(request, "storage-second");

  await page.goto(`/?token=${first.token}`);
  await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });
  const firstWorkspace = await getWorkspaceId(page);
  await expect(page.getByTestId("user-menu-button")).toHaveAttribute("title", "storage-first Test User");

  const otherTab = await context.newPage();
  await otherTab.goto(`/?token=${second.token}`);
  await expect(otherTab.locator("aside")).toBeVisible({ timeout: 15000 });
  const secondWorkspace = await getWorkspaceId(otherTab);
  expect(secondWorkspace).not.toBe(firstWorkspace);

  await expect(page.getByTestId("user-menu-button")).toHaveAttribute("title", "storage-second Test User");
  await expect(page).toHaveURL(new RegExp(`/w/@${secondWorkspace}(?:[+/]|$)`));
  await otherTab.close();
});
