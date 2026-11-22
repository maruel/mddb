// Exercises low workspace storage rejection, retained drafts, and shrinking recovery through the editor.

import { test, expect, registerUser, createClient, gotoWithSession, waitForWorkspace } from "./helpers";

test("workspace quota rejection preserves saved data and allows shrinking recovery", async ({ page, request }) => {
  const { token } = await registerUser(request, "workspace-quota-recovery");
  await gotoWithSession(page, token);
  const wsID = await waitForWorkspace(page);
  const ws = createClient(request, token).ws(wsID);
  const source = await ws.createPage("0", { title: "Original title with room to shrink", content: "Original body" });
  const config = await ws.getWorkspace();
  await ws.updateWorkspace({ quotas: { ...config.quotas, max_storage_bytes: 0 } });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  await expect(title).toHaveValue("Original title with room to shrink");
  // Read the server response before the browser SDK cancels retryable bodies.
  await page.route(`**/nodes/${source.id}/page`, async (route) => {
    const response = await route.fetch();
    if (response.status() === 429) {
      expect(await response.json()).toMatchObject({ error: { code: "QUOTA_EXCEEDED" } });
    }
    await route.fulfill({ response });
  });
  const rejected = page.waitForResponse((r) => r.url().endsWith(`/nodes/${source.id}/page`) && r.status() === 429);
  await title.fill("A much larger title that exceeds the workspace storage allowance and must remain a draft");
  await rejected;
  // The existing browser transport retries 429 at 1s/2s/4s before surfacing it.
  await expect(page.getByRole("status").filter({ hasText: "Auto-save failed" })).toBeVisible({ timeout: 12000 });
  expect((await ws.getNode(source.id)).title).toBe("Original title with room to shrink");
  const saved = page.waitForResponse((r) => r.url().endsWith(`/nodes/${source.id}/page`) && r.status() === 200);
  await title.fill("Short");
  await saved;
  await expect.poll(async () => (await ws.getNode(source.id)).title).toBe("Short");
  await expect(page.getByRole("status").filter({ hasText: "Auto-save failed" })).not.toBeVisible();
  await ws.deletePage(source.id);
});
