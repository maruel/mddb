// Exercises editor representation, undo, and refresh ownership while uploads or departure are pending.

import type { Page } from "@playwright/test";
import { test, expect, registerUser, createClient, waitForWorkspace, gotoWithSession } from "./helpers";
import { gate } from "./gomode-fixtures";

async function dropImage(page: Page) {
  const transfer = await page.evaluateHandle(() => {
    const data = new DataTransfer();
    data.items.add(
      new File(['<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"/>'], "held.svg", {
        type: "image/svg+xml",
      }),
    );
    return data;
  });
  await page.locator(".ProseMirror").dispatchEvent("drop", { dataTransfer: transfer });
  await transfer.dispose();
}

test("an upload and waiting departure preserve typing across attempted mode changes", async ({ page, request }) => {
  const { token } = await registerUser(request, "upload-mode-ownership");
  await gotoWithSession(page, token);
  const wsID = await waitForWorkspace(page);
  const ws = createClient(request, token).ws(wsID);
  const source = await ws.createPage("0", { title: "Mode source", content: "Original body" });
  const target = await ws.createPage("0", { title: "Mode target", content: "Other body" });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  await expect(title).toHaveValue("Mode source");
  const uploaded = gate(),
    reply = gate();
  await page.route(`**/nodes/${source.id}/assets`, async (route) => {
    const response = await route.fetch();
    uploaded.resolve();
    await reply.promise;
    await route.fulfill({ response });
  });
  try {
    await dropImage(page);
    await uploaded.promise;
    await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
    await page.keyboard.press("Control+Shift+M");
    // Exercise the active representation, including Markdown on the broken path.
    await page.locator('[data-testid="markdown-editor"]:visible, .ProseMirror:visible').fill("NEW MARKDOWN DRAFT");
    await expect(page.getByTestId("editor-mode-markdown")).toBeDisabled();
    await expect(page.getByTestId("editor-mode-visual")).toBeDisabled();
    reply.resolve();
    await expect(title).toHaveValue("Mode target");
    await expect(page.getByTestId("editor-mode-markdown")).toBeEnabled();
    const saved = await ws.getNode(source.id);
    expect(saved.content).toContain("NEW MARKDOWN DRAFT");
    expect(saved.content).toContain("held.svg");
  } finally {
    reply.resolve();
  }
});

test("locked global mode shortcuts and same-mode clicks preserve undo after failed deletion", async ({
  page,
  request,
}) => {
  const { token } = await registerUser(request, "locked-mode-undo");
  await gotoWithSession(page, token);
  const wsID = await waitForWorkspace(page);
  const ws = createClient(request, token).ws(wsID);
  const source = await ws.createPage("0", { title: "Undo source", content: "Original body" });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  const editor = page.locator(".ProseMirror");
  await expect(title).toHaveValue("Undo source");
  await editor.fill("Typed before deletion");
  const deleting = gate(),
    reply = gate();
  await page.route(`**/nodes/${source.id}/delete`, async (route) => {
    deleting.resolve();
    await reply.promise;
    await route.fulfill({ status: 400, json: { error: { code: "VALIDATION_FAILED", message: "Deletion rejected" } } });
  });
  try {
    page.once("dialog", (dialog) => dialog.accept());
    const row = page.getByTestId(`sidebar-node-${source.id}`);
    await row.hover();
    await row.getByTestId("delete-node-button").click();
    await deleting.promise;
    await expect(title).toBeDisabled();
    await page.keyboard.press("Control+Shift+M");
    await page.keyboard.press("Control+Shift+M");
    reply.resolve();
    await expect(title).toBeEnabled();
    await editor.click();
    await page.keyboard.press("Control+z");
    await expect(editor).toHaveText("Original body");
    await page.keyboard.press("Control+Shift+z");
    await expect(editor).toHaveText("Typed before deletion");
    await page.getByTestId("editor-mode-visual").click();
    await editor.click();
    await page.keyboard.press("Control+z");
    await expect(editor).toHaveText("Original body");
  } finally {
    reply.resolve();
  }
});

test("an admitted upload prevents an external refresh from replacing its source draft", async ({ page, request }) => {
  const { token } = await registerUser(request, "upload-refresh-ownership");
  await gotoWithSession(page, token);
  const wsID = await waitForWorkspace(page);
  const ws = createClient(request, token).ws(wsID);
  const source = await ws.createPage("0", { title: "Refresh source", content: "Original body" });
  const target = await ws.createPage("0", { title: "Refresh target", content: "Other body" });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  await expect(title).toHaveValue("Refresh source");
  await expect(page.getByTestId("connection-status")).toHaveText("Connected");
  const refreshing = gate(),
    refreshReply = gate(),
    refreshed = gate(),
    uploaded = gate(),
    uploadReply = gate();
  await page.route(`**/nodes/${source.id}`, async (route) => {
    const response = await route.fetch();
    refreshing.resolve();
    await refreshReply.promise;
    await route.fulfill({ response });
    refreshed.resolve();
  });
  await page.route(`**/nodes/${source.id}/assets`, async (route) => {
    const response = await route.fetch();
    uploaded.resolve();
    await uploadReply.promise;
    await route.fulfill({ response });
  });
  try {
    await ws.updatePage(source.id, {
      title: "External replacement",
      content: "External body",
      expected_revision: (await ws.getNode(source.id)).revision ?? "",
    });
    await refreshing.promise;
    await dropImage(page);
    await uploaded.promise;
    refreshReply.resolve();
    await refreshed.promise;
    await expect(page.getByText("This page changed elsewhere.", { exact: false })).toBeVisible();
    await expect(title).toHaveValue("Refresh source");
    await expect(page.getByRole("button", { name: "Refresh", exact: true })).toBeDisabled();
    uploadReply.resolve();
    await expect(page.locator('.ProseMirror img[alt="held.svg"]')).toBeVisible();
    await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
    await expect(page.getByRole("button", { name: "Copy draft", exact: true })).toBeVisible();
    await expect(page.locator(".ProseMirror")).toContainText("Original body");
    expect((await ws.getNode(source.id)).content).toBe("External body");
  } finally {
    refreshReply.resolve();
    uploadReply.resolve();
  }
});

test("a locked Markdown draft cannot be round-tripped by the global shortcut", async ({ page, request }) => {
  const { token } = await registerUser(request, "locked-markdown-representation");
  await gotoWithSession(page, token);
  const wsID = await waitForWorkspace(page);
  const ws = createClient(request, token).ws(wsID);
  const source = await ws.createPage("0", { title: "Markdown deletion source", content: "Original body" });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  await expect(title).toHaveValue("Markdown deletion source");
  await page.getByTestId("editor-mode-markdown").click();
  const markdown = page.getByTestId("markdown-editor");
  const draft = "# Markdown draft\n\nLiteral spacing   \n\n";
  await markdown.fill(draft);
  const deleting = gate(),
    reply = gate();
  await page.route(`**/nodes/${source.id}/delete`, async (route) => {
    deleting.resolve();
    await reply.promise;
    await route.fulfill({ status: 400, json: { error: { code: "VALIDATION_FAILED", message: "Deletion rejected" } } });
  });
  try {
    page.once("dialog", (dialog) => dialog.accept());
    const row = page.getByTestId(`sidebar-node-${source.id}`);
    await row.hover();
    await row.getByTestId("delete-node-button").click();
    await deleting.promise;
    await page.keyboard.press("Control+Shift+M");
    await page.keyboard.press("Control+Shift+M");
    await expect(markdown).toBeVisible();
    await expect(markdown).toHaveValue(draft);
    await expect(page.getByTestId("editor-mode-visual")).toBeDisabled();
    reply.resolve();
    await expect(title).toBeEnabled();
    await expect(markdown).toHaveValue(draft);
  } finally {
    reply.resolve();
  }
});

for (const { outcome, stage, uploadFirst } of [
  { outcome: "navigate", stage: "preview", uploadFirst: false },
  { outcome: "navigate", stage: "preview", uploadFirst: true },
  { outcome: "navigate", stage: "list", uploadFirst: false },
  { outcome: "navigate", stage: "list", uploadFirst: true },
  { outcome: "cancel", stage: "preview", uploadFirst: false },
  { outcome: "fail", stage: "preview", uploadFirst: false },
  { outcome: "workspace", stage: "preview", uploadFirst: false },
] as const) {
  test(`a ${outcome} History ${stage} response preserves uploads (${uploadFirst ? "upload first" : "History first"})`, async ({
    page,
    request,
  }) => {
    const { token } = await registerUser(request, `history-upload-${outcome}`);
    await gotoWithSession(page, token);
    const wsID = await waitForWorkspace(page);
    const client = createClient(request, token);
    const ws = client.ws(wsID);
    const other =
      outcome === "workspace"
        ? await client
            .org((await client.getMe()).organization_id!)
            .createWorkspace({ name: "History destination workspace" })
        : null;
    const source = await ws.createPage("0", { title: "History upload source", content: "Original body" });
    const target = await ws.createPage("0", { title: "History upload target", content: "Other body" });
    await page.goto(`/w/@${wsID}/@${source.id}`);
    const title = page.getByRole("textbox", { name: "Title", exact: true });
    await expect(title).toHaveValue("History upload source");
    const previewStarted = gate(),
      previewReply = gate(),
      uploaded = gate(),
      uploadReply = gate();
    const isHistoryRead = (url: URL) =>
      stage === "list"
        ? url.pathname.endsWith(`/nodes/${source.id}/history`)
        : url.pathname.includes(`/nodes/${source.id}/history/`);
    await page.route(isHistoryRead, async (route) => {
      const response = await route.fetch();
      previewStarted.resolve();
      await previewReply.promise;
      if (outcome === "fail")
        await route.fulfill({
          status: 400,
          json: { error: { code: "VALIDATION_FAILED", message: "History version unavailable" } },
        });
      else await route.fulfill({ response });
    });
    await page.route(`**/nodes/${source.id}/assets`, async (route) => {
      const response = await route.fetch();
      uploaded.resolve();
      await uploadReply.promise;
      await route.fulfill({ response });
    });
    const historyAction = async () => {
      await page.getByTestId(`sidebar-node-${source.id}`).click({ button: "right" });
      await page.getByTestId("context-menu-history").click();
    };
    const settle = () =>
      page.evaluate(
        () => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))),
      );
    try {
      if (outcome === "cancel" || outcome === "fail") await page.locator(".ProseMirror").fill("Typed before History");
      await historyAction();
      await previewStarted.promise;
      await dropImage(page);
      await uploaded.promise;
      if (outcome === "cancel") await historyAction();
      if (other) {
        await page.getByRole("button", { name: "Other workspaces", exact: true }).click();
        await page.getByRole("button", { name: "History destination workspace", exact: true }).click();
        uploadReply.resolve();
        await expect(page).toHaveURL(new RegExp(`/w/@${other.id}`));
      } else if (outcome === "navigate") {
        await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
      }
      if (uploadFirst) {
        const uploadDelivered = page.waitForResponse((response) =>
          new URL(response.url()).pathname.endsWith(`/nodes/${source.id}/assets`),
        );
        uploadReply.resolve();
        await uploadDelivered;
        // Prove the first attempt can finish before the obsolete History read.
        await expect(title).toHaveValue("History upload target");
      }
      const delivered = page.waitForResponse((response) => isHistoryRead(new URL(response.url())));
      previewReply.resolve();
      await delivered;
      await settle();
      if (!other) {
        if (outcome === "cancel" || outcome === "fail") {
          await page.locator(".ProseMirror").click();
          await page.keyboard.press("Control+z");
          await expect(page.locator(".ProseMirror")).toHaveText("Original body");
          await page.keyboard.press("Control+Shift+z");
          await expect(page.locator(".ProseMirror")).toHaveText("Typed before History");
        }
        if (!uploadFirst) {
          const uploadDelivered = page.waitForResponse((response) =>
            new URL(response.url()).pathname.endsWith(`/nodes/${source.id}/assets`),
          );
          uploadReply.resolve();
          await uploadDelivered;
          await settle();
        }
        // The original navigation must complete without another user attempt.
        if (outcome !== "navigate")
          await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
        await expect(title).toHaveValue("History upload target");
      }
      const saved = await ws.getNode(source.id);
      expect(saved.content).toContain(
        outcome === "cancel" || outcome === "fail" ? "Typed before History" : "Original body",
      );
      expect(saved.content).toContain("held.svg");
      await expect(page.getByRole("region", { name: "Markdown preview" })).toBeHidden();
      if (!other) {
        await page.getByTestId(`sidebar-node-${source.id}`).locator("> div").first().click();
        await expect(title).toHaveValue("History upload source");
        await title.fill("Editable after History");
        await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
        await expect(title).toHaveValue("History upload target");
        expect((await ws.getNode(source.id)).title).toBe("Editable after History");
      }
    } finally {
      previewReply.resolve();
      uploadReply.resolve();
    }
  });
}

for (const destination of ["same", "other"] as const) {
  test(`sidebar History for the ${destination} node drains an upload before retiring its editor`, async ({
    page,
    request,
  }) => {
    const { token } = await registerUser(request, `sidebar-history-upload-${destination}`);
    await gotoWithSession(page, token);
    const wsID = await waitForWorkspace(page);
    const ws = createClient(request, token).ws(wsID);
    const source = await ws.createPage("0", { title: "Sidebar History source", content: "Original body" });
    const target = await ws.createPage("0", { title: "Sidebar History target", content: "Other body" });
    await page.goto(`/w/@${wsID}/@${source.id}`);
    await expect(page.getByRole("textbox", { name: "Title", exact: true })).toHaveValue("Sidebar History source");
    const uploaded = gate(),
      uploadReply = gate(),
      loadingTarget = gate(),
      targetReply = gate();
    await page.route(`**/nodes/${source.id}/assets`, async (route) => {
      const response = await route.fetch();
      uploaded.resolve();
      await uploadReply.promise;
      await route.fulfill({ response });
    });
    await page.route(`**/nodes/${target.id}`, async (route) => {
      const response = await route.fetch();
      loadingTarget.resolve();
      await targetReply.promise;
      await route.fulfill({ response });
    });
    try {
      await dropImage(page);
      await uploaded.promise;
      await page
        .getByTestId(`sidebar-node-${destination === "same" ? source.id : target.id}`)
        .click({ button: "right" });
      await page.getByTestId("context-menu-history").click();
      if (destination === "same") {
        await expect(page.getByRole("heading", { name: "Version History", exact: true })).toBeVisible();
        await expect(page.locator(".ProseMirror")).toBeVisible();
      }
      uploadReply.resolve();
      if (destination === "other") {
        await loadingTarget.promise;
        expect((await ws.getNode(source.id)).content).toContain("held.svg");
        targetReply.resolve();
      }
      await expect(page.getByRole("region", { name: "Markdown preview" })).toContainText(
        destination === "same" ? "Original body" : "Other body",
      );
      expect((await ws.getNode(source.id)).content).toContain("held.svg");
      await page.getByRole("button", { name: "Close Preview", exact: true }).click();
      await expect(page.locator(".ProseMirror")).toBeVisible();
      await expect(page.getByTestId("editor-mode-markdown")).toBeEnabled();
      if (destination === "same") await expect(page.locator('.ProseMirror img[alt="held.svg"]')).toBeVisible();
    } finally {
      uploadReply.resolve();
      targetReply.resolve();
    }
  });
}

for (const held of ["create", "refresh"] as const) {
  test(`a sidebar creation does not steal a later route after its ${held} response`, async ({ page, request }) => {
    const { token } = await registerUser(request, `sidebar-create-intent-${held}`);
    await gotoWithSession(page, token);
    const wsID = await waitForWorkspace(page);
    const ws = createClient(request, token).ws(wsID);
    const source = await ws.createPage("0", { title: "Sidebar create source" });
    const target = await ws.createPage("0", { title: "Sidebar chosen target" });
    await page.goto(`/w/@${wsID}/@${source.id}`);
    const title = page.getByRole("textbox", { name: "Title", exact: true });
    await expect(title).toHaveValue("Sidebar create source");
    const created = gate(),
      createReply = gate(),
      refreshing = gate(),
      refreshReply = gate();
    let childID = "";
    await page.route(`**/nodes/${source.id}/page/create`, async (route) => {
      const response = await route.fetch();
      childID = (await response.json()).id;
      created.resolve();
      if (held === "create") await createReply.promise;
      await route.fulfill({ response });
    });
    if (held === "refresh")
      await page.route(`**/nodes/0/children`, async (route) => {
        const response = await route.fetch();
        refreshing.resolve();
        await refreshReply.promise;
        await route.fulfill({ response });
      });
    try {
      await page.getByTestId(`sidebar-node-${source.id}`).click({ button: "right" });
      await page.getByTestId("context-menu-create-subpage").click();
      await created.promise;
      if (held === "refresh") await refreshing.promise;
      await page.getByRole("button", { name: "Toggle menu", exact: true }).click();
      await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
      await expect(title).toHaveValue("Sidebar chosen target");
      const delivered = page.waitForResponse((response) =>
        new URL(response.url()).pathname.endsWith(
          held === "create" ? `/nodes/${source.id}/page/create` : "/nodes/0/children",
        ),
      );
      createReply.resolve();
      refreshReply.resolve();
      await delivered;
      // Observe the completed follow-up through the real tree, not just a network reply.
      const expand = page.getByTestId(`expand-icon-${source.id}`);
      await expect(expand).toBeVisible();
      if ((await expand.getAttribute("title")) === "Expand Sidebar create source") await expand.click();
      await expect(page.getByTestId(`sidebar-node-${childID}`)).toBeVisible();
      await expect(title).toHaveValue("Sidebar chosen target");
      expect((await ws.getNode(childID)).parent_id).toBe(source.id);
    } finally {
      createReply.resolve();
      refreshReply.resolve();
    }
  });
}

test("a selected deletion cannot replace a newer destination whose content is still loading", async ({
  page,
  request,
}) => {
  const { token } = await registerUser(request, "delete-route-intent");
  await gotoWithSession(page, token);
  const wsID = await waitForWorkspace(page);
  const ws = createClient(request, token).ws(wsID);
  const source = await ws.createPage("0", { title: "Deleting source" });
  const target = await ws.createPage("0", { title: "Chosen after deletion", content: "Chosen destination body" });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  await expect(title).toHaveValue("Deleting source");
  const deleting = gate(),
    deletionReply = gate(),
    loading = gate(),
    destinationReply = gate();
  await page.route(`**/nodes/${source.id}/delete`, async (route) => {
    const response = await route.fetch();
    deleting.resolve();
    await deletionReply.promise;
    await route.fulfill({ response });
  });
  await page.route(`**/nodes/${target.id}`, async (route) => {
    loading.resolve();
    await destinationReply.promise;
    await route.continue();
  });
  try {
    page.once("dialog", (dialog) => dialog.accept());
    const sourceItem = page.getByTestId(`sidebar-node-${source.id}`);
    await sourceItem.hover();
    await sourceItem.getByTestId("delete-node-button").click();
    await deleting.promise;
    await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
    await loading.promise;
    const delivered = page.waitForResponse((response) =>
      new URL(response.url()).pathname.endsWith(`/nodes/${source.id}/delete`),
    );
    deletionReply.resolve();
    await delivered;
    await page.evaluate(
      () => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))),
    );
    await expect(sourceItem).toHaveCount(0);
    destinationReply.resolve();
    await expect(title).toHaveValue("Chosen after deletion");
    await expect(page.locator(".ProseMirror")).toHaveText("Chosen destination body");
  } finally {
    deletionReply.resolve();
    destinationReply.resolve();
  }
});
