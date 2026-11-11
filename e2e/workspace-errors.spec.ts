// Exercises workspace command failures, acknowledged mutation recovery, and guarded success continuations.

import { test, expect, registerUser, createClient, gotoWithSession, waitForWorkspace } from "./helpers";
import type { ErrorResponse, NotionImportResponse, NotionImportStatusResponse } from "../sdk/mddb/ts/v1/types.gen";

const denied = (message: string): ErrorResponse => ({ error: { code: "FORBIDDEN", message } });

for (const kind of ["creation", "import"] as const) {
  test(`acknowledged workspace ${kind} retries opening without repeating the mutation`, async ({ page, request }) => {
    const { token } = await registerUser(request, `command-${kind}`);
    const client = createClient(request, token);
    await gotoWithSession(page, token);
    const previous = await waitForWorkspace(page);
    const me = await client.getMe();
    if (!me.organization_id) throw new Error("Expected organization");
    const imported =
      kind === "import" ? await client.org(me.organization_id).createWorkspace({ name: "Imported workspace" }) : null;
    let mutationCount = 0;
    let createdId = "";
    if (kind === "creation") {
      await page.route("**/api/v1/organizations/*/workspaces", async (route) => {
        mutationCount++;
        const response = await route.fetch();
        const workspace = await response.json();
        expect(typeof workspace.id).toBe("string");
        createdId = workspace.id;
        await route.fulfill({ response });
      });
    } else {
      if (!imported) throw new Error("Expected import workspace");
      createdId = imported.id;
      await page.route("**/notion/import", async (route) => {
        mutationCount++;
        await route.fulfill({
          json: {
            workspace_id: imported.id,
            workspace_name: "Imported workspace",
            status: "running",
          } satisfies NotionImportResponse,
        });
      });
      await page.route("**/notion/import/*/status", (route) =>
        route.fulfill({ json: { status: "completed", progress: 1, total: 1 } satisfies NotionImportStatusResponse }),
      );
    }
    let switches = 0;
    await page.route("**/api/v1/auth/switch-workspace", async (route) => {
      switches++;
      if (switches === 1) await route.fulfill({ status: 403, json: denied("Opening denied") });
      else await route.continue();
    });
    await page.getByTestId(kind === "creation" ? "create-workspace-button" : "import-notion-button").click();
    const dialog = page.getByRole("dialog", {
      name: kind === "creation" ? "Create Workspace" : "Import from Notion",
      exact: true,
    });
    await dialog
      .getByLabel(kind === "creation" ? "Workspace Name" : "Internal Integration Secret")
      .fill(kind === "creation" ? "Created once" : "private-token");
    await dialog.getByRole("button", { name: kind === "creation" ? "Create" : "Start Import", exact: true }).click();
    await expect(dialog.getByText(/Opening denied/)).toBeVisible();
    await expect(dialog.getByRole("status")).toContainText(
      kind === "creation" ? "Workspace created" : "Import started",
    );
    await expect(page).toHaveURL(new RegExp(`/w/@${previous}`));
    expect(mutationCount).toBe(1);
    const retry = dialog.getByRole("button", { name: "Try again", exact: true });
    await retry.focus();
    await page.keyboard.press("Enter");
    await expect(dialog).not.toBeVisible();
    await expect(page).toHaveURL(new RegExp(`/w/@${createdId}`));
    await waitForWorkspace(page);
    expect(mutationCount).toBe(1);
    expect(switches).toBe(2);
  });
}

test("an acknowledged page creation keeps its document when canonical root refresh fails", async ({
  page,
  request,
}) => {
  const { token } = await registerUser(request, "command-create-refresh");
  const client = createClient(request, token);
  await gotoWithSession(page, token);
  const wsId = await waitForWorkspace(page);
  const parent = await client.ws(wsId).createPage("0", { title: "Creation parent", content: "Parent body" });
  await page.goto(`/w/@${wsId}/@${parent.id}`);
  await expect(page.getByRole("textbox", { name: "Title", exact: true })).toHaveValue("Creation parent");
  await waitForWorkspace(page);
  let createdId = "";
  let creates = 0;
  await page.route(`**/nodes/${parent.id}/page/create`, async (route) => {
    creates++;
    const response = await route.fetch();
    createdId = (await response.json()).id;
    await route.fulfill({ response });
  });
  await page.route("**/nodes/0/children", (route) =>
    route.fulfill({ status: 403, json: denied("Root refresh denied") }),
  );
  await page.getByRole("treeitem", { name: "Creation parent", exact: true }).click({ button: "right" });
  await page.getByTestId("context-menu-create-subpage").click();
  await expect(page.getByRole("alert")).toContainText("Root refresh denied");
  await expect(page).toHaveURL(new RegExp(`/@${createdId}`));
  await expect(page.getByRole("textbox", { name: "Title", exact: true })).not.toHaveValue("Creation parent");
  expect(creates).toBe(1);
  expect((await client.ws(wsId).getNode(createdId)).parent_id).toBe(parent.id);
  await page.unroute("**/nodes/0/children");
  await page.getByRole("alert").getByRole("button", { name: "Try again", exact: true }).click();
  await expect(page.getByRole("alert")).not.toBeVisible();
  await waitForWorkspace(page);
  expect(creates).toBe(1);
});

test("a failed child refresh stops subpage departure without losing the saved parent link", async ({
  page,
  request,
}) => {
  const { token } = await registerUser(request, "command-child-refresh");
  const client = createClient(request, token);
  await gotoWithSession(page, token);
  const wsId = await waitForWorkspace(page);
  const parent = await client.ws(wsId).createPage("0", { title: "Subpage parent", content: "" });
  await page.goto(`/w/@${wsId}/@${parent.id}`);
  await expect(page.getByRole("textbox", { name: "Title", exact: true })).toHaveValue("Subpage parent");
  await waitForWorkspace(page);
  let createdId = "";
  await page.route(`**/nodes/${parent.id}/page/create`, async (route) => {
    const response = await route.fetch();
    createdId = (await response.json()).id;
    await route.fulfill({ response });
  });
  await page.route(`**/nodes/${parent.id}/children`, (route) =>
    route.fulfill({ status: 403, json: denied("Child refresh denied") }),
  );
  await page.locator(".ProseMirror").click();
  await page.keyboard.type("/page");
  await expect(page.getByTestId("slash-command-menu")).toBeVisible();
  await page.keyboard.press("Enter");
  await expect(page.getByTestId("workspace-feedback")).toContainText("Child refresh denied");
  await expect(page).toHaveURL(new RegExp(`/@${parent.id}`));
  await expect(page.getByRole("textbox", { name: "Title", exact: true })).toHaveValue("Subpage parent");
  expect((await client.ws(wsId).getNode(parent.id)).content).toContain(createdId);
  expect((await client.ws(wsId).getNode(createdId)).parent_id).toBe(parent.id);
  await page.unroute(`**/nodes/${parent.id}/children`);
});

test("a rejected move leaves hierarchy and selected document unchanged with one visible error", async ({
  page,
  request,
}) => {
  const { token } = await registerUser(request, "command-move");
  const client = createClient(request, token);
  await gotoWithSession(page, token);
  const wsId = await waitForWorkspace(page);
  const source = await client.ws(wsId).createPage("0", { title: "Move source", content: "Private source" });
  const destination = await client.ws(wsId).createPage("0", { title: "Move destination", content: "Destination" });
  await page.goto(`/w/@${wsId}/@${source.id}`);
  await expect(page.getByRole("textbox", { name: "Title", exact: true })).toHaveValue("Move source");
  await waitForWorkspace(page);
  await page.route(`**/nodes/${source.id}/move`, (route) =>
    route.fulfill({ status: 403, json: denied("Move denied") }),
  );
  await page.getByRole("treeitem", { name: "Move source", exact: true }).click({ button: "right" });
  await page.getByTestId("context-menu-move-node").click();
  await page.getByRole("treeitem", { name: "Move destination", exact: true }).focus();
  await page.keyboard.press("Control+v");
  await expect(page.getByTestId("workspace-feedback")).toContainText("Move denied");
  await expect(page.getByRole("alert")).toHaveCount(1);
  await expect(page).toHaveURL(new RegExp(`/@${source.id}`));
  await expect(page.getByRole("textbox", { name: "Title", exact: true })).toHaveValue("Move source");
  expect((await client.ws(wsId).getNode(source.id)).parent_id ?? "0").toBe("0");
  await expect(page.getByRole("treeitem", { name: "Move source", exact: true })).toHaveAttribute("aria-level", "1");
  await expect(page.getByRole("treeitem", { name: "Move destination", exact: true })).toHaveAttribute(
    "data-node-id",
    destination.id,
  );
});

test("German acknowledged-workspace recovery stays usable at narrow width", async ({ page, request }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const { token } = await registerUser(request, "command-create-de");
  const client = createClient(request, token);
  await client.updateUserSettings({ settings: { theme: "system", language: "de" } });
  await gotoWithSession(page, token);
  await waitForWorkspace(page);
  await page.route("**/api/v1/auth/switch-workspace", (route) =>
    route.fulfill({ status: 403, json: denied("Öffnen nicht erlaubt") }),
  );
  const create = page.getByTestId("create-workspace-button");
  if (!(await create.isVisible())) await page.getByRole("button", { name: "Toggle menu" }).click();
  await create.click();
  const dialog = page.getByRole("dialog", { name: "Arbeitsbereich erstellen", exact: true });
  await dialog.getByLabel("Name des Arbeitsbereichs").fill("Ein neuer Arbeitsbereich");
  await dialog.getByRole("button", { name: "Erstellen", exact: true }).click();
  await expect(dialog.getByText(/Öffnen nicht erlaubt/)).toBeVisible();
  await expect(dialog.getByRole("status")).toContainText("Arbeitsbereich erstellt");
  const retry = dialog.getByRole("button", { name: "Erneut versuchen", exact: true });
  await retry.focus();
  await expect(retry).toBeFocused();
  const bounds = await retry.boundingBox();
  if (!bounds) throw new Error("Expected visible retry bounds");
  expect(bounds.x).toBeGreaterThanOrEqual(0);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(390);
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(844);
  await page.screenshot({ path: "/tmp/mddb-workspace-errors-de-narrow.png" });
});
