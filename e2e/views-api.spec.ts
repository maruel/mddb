// E2E tests for table views API: filters, sorts, and view CRUD.

import { test, expect, registerUser, waitForWorkspace, createClient, gotoWithSession } from "./helpers";
import { nodeUrl } from "../frontend/src/utils/urls";
import type { DataRecordResponse } from "../sdk/mddb/ts/v1/types.gen";

test.describe("Table Views API", () => {
  test("create view and list records with filter", async ({ page, request }) => {
    const { token } = await registerUser(request, "views-api");
    const client = createClient(request, token);

    await gotoWithSession(page, token);
    const wsID = await waitForWorkspace(page);

    // Create Table
    const tableData = await client.ws(wsID).createTable("0", {
      title: "Views Test Table",
      properties: [
        { name: "Name", type: "text" },
        { name: "Age", type: "number" },
      ],
    });
    const tableID = tableData.id;

    await client.ws(wsID).createRecord(tableID, { data: { Name: "Alice", Age: 25 } });
    await client.ws(wsID).createRecord(tableID, { data: { Name: "Bob", Age: 10 } });

    // Verify all records returned by default
    const defaultListParams = { ViewID: "", Filters: "", Sorts: "", Offset: 0, Limit: 100 };
    const listData = await client.ws(wsID).listRecords(tableID, defaultListParams);
    expect(listData.records?.length).toBe(2);

    // Create a View with filter (Age > 18)
    const viewData = await client.ws(wsID).createView(tableID, { name: "Adults", type: "table" });
    const viewID = viewData.view.id;
    await client.ws(wsID).updateView(tableID, viewID, {
      expected_revision: viewData.view.revision,
      filters: [{ property: "Age", operator: "gt", value: 18 }],
    });

    // List with ViewID → only Alice
    const viewListParams = { ViewID: viewID, Filters: "", Sorts: "", Offset: 0, Limit: 100 };
    const listViewData = await client.ws(wsID).listRecords(tableID, viewListParams);
    expect(listViewData.records?.length).toBe(1);
    expect((listViewData.records?.[0] as DataRecordResponse).data?.Name).toBe("Alice");

    // Ad-hoc filter (Age < 15) → only Bob
    const filterListParams = {
      ViewID: "",
      Filters: JSON.stringify([{ property: "Age", operator: "lt", value: 15 }]),
      Sorts: "",
      Offset: 0,
      Limit: 100,
    };
    const listAdHocData = await client.ws(wsID).listRecords(tableID, filterListParams);
    expect(listAdHocData.records?.length).toBe(1);
    expect((listAdHocData.records?.[0] as DataRecordResponse).data?.Name).toBe("Bob");
  });

  test("sort records via API", async ({ page, request }) => {
    const { token } = await registerUser(request, "views-sort-api");
    const client = createClient(request, token);

    await gotoWithSession(page, token);
    const wsID = await waitForWorkspace(page);

    const tableData = await client.ws(wsID).createTable("0", {
      title: "Sort API Table",
      properties: [
        { name: "Name", type: "text" },
        { name: "Score", type: "number" },
      ],
    });
    const tableID = tableData.id;

    await client.ws(wsID).createRecord(tableID, { data: { Name: "Charlie", Score: 50 } });
    await client.ws(wsID).createRecord(tableID, { data: { Name: "Alice", Score: 90 } });
    await client.ws(wsID).createRecord(tableID, { data: { Name: "Bob", Score: 70 } });

    // Sort by Name asc
    const nameAscParams = {
      ViewID: "",
      Filters: "",
      Sorts: JSON.stringify([{ property: "Name", direction: "asc" }]),
      Offset: 0,
      Limit: 100,
    };
    const nameAsc = await client.ws(wsID).listRecords(tableID, nameAscParams);
    const namesAsc = nameAsc.records?.map((r: DataRecordResponse) => r.data?.Name);
    expect(namesAsc).toEqual(["Alice", "Bob", "Charlie"]);

    // Sort by Score desc
    const scoreDescParams = {
      ViewID: "",
      Filters: "",
      Sorts: JSON.stringify([{ property: "Score", direction: "desc" }]),
      Offset: 0,
      Limit: 100,
    };
    const scoreDesc = await client.ws(wsID).listRecords(tableID, scoreDescParams);
    const scoresDesc = scoreDesc.records?.map((r: DataRecordResponse) => r.data?.Name);
    expect(scoresDesc).toEqual(["Alice", "Bob", "Charlie"]);

    // Compound sort: Score asc, then Name asc (add ties)
    await client.ws(wsID).createRecord(tableID, { data: { Name: "Diana", Score: 70 } });
    const compoundParams = {
      ViewID: "",
      Filters: "",
      Sorts: JSON.stringify([
        { property: "Score", direction: "asc" },
        { property: "Name", direction: "asc" },
      ]),
      Offset: 0,
      Limit: 100,
    };
    const compound = await client.ws(wsID).listRecords(tableID, compoundParams);
    const compoundNames = compound.records?.map((r: DataRecordResponse) => r.data?.Name);
    expect(compoundNames).toEqual(["Charlie", "Bob", "Diana", "Alice"]);
  });

  test("view persists sorts across requests", async ({ page, request }) => {
    const { token } = await registerUser(request, "views-sort-persist");
    const client = createClient(request, token);

    await gotoWithSession(page, token);
    const wsID = await waitForWorkspace(page);

    const tableData = await client.ws(wsID).createTable("0", {
      title: "Sort Persist Table",
      properties: [{ name: "Name", type: "text" }],
    });
    const tableID = tableData.id;

    await client.ws(wsID).createRecord(tableID, { data: { Name: "Zebra" } });
    await client.ws(wsID).createRecord(tableID, { data: { Name: "Apple" } });

    // Create view with sort
    const view = await client.ws(wsID).createView(tableID, { name: "Sorted", type: "table" });
    await client.ws(wsID).updateView(tableID, view.view.id, {
      expected_revision: view.view.revision,
      sorts: [{ property: "Name", direction: "asc" }],
    });

    // Query with ViewID (sort applied server-side)
    const params = { ViewID: view.view.id, Filters: "", Sorts: "", Offset: 0, Limit: 100 };
    const data = await client.ws(wsID).listRecords(tableID, params);
    const names = data.records?.map((r: DataRecordResponse) => r.data?.Name);
    expect(names).toEqual(["Apple", "Zebra"]);
  });

  test("filter and sort combined", async ({ page, request }) => {
    const { token } = await registerUser(request, "views-filter-sort");
    const client = createClient(request, token);

    await gotoWithSession(page, token);
    const wsID = await waitForWorkspace(page);

    const tableData = await client.ws(wsID).createTable("0", {
      title: "Filter Sort Table",
      properties: [
        { name: "Name", type: "text" },
        { name: "Age", type: "number" },
      ],
    });
    const tableID = tableData.id;

    await client.ws(wsID).createRecord(tableID, { data: { Name: "Charlie", Age: 30 } });
    await client.ws(wsID).createRecord(tableID, { data: { Name: "Alice", Age: 25 } });
    await client.ws(wsID).createRecord(tableID, { data: { Name: "Bob", Age: 10 } });
    await client.ws(wsID).createRecord(tableID, { data: { Name: "Diana", Age: 22 } });

    // Filter: Age >= 20, Sort: Name desc
    const params = {
      ViewID: "",
      Filters: JSON.stringify([{ property: "Age", operator: "gte", value: 20 }]),
      Sorts: JSON.stringify([{ property: "Name", direction: "desc" }]),
      Offset: 0,
      Limit: 100,
    };
    const data = await client.ws(wsID).listRecords(tableID, params);
    const names = data.records?.map((r: DataRecordResponse) => r.data?.Name);
    expect(names).toEqual(["Diana", "Charlie", "Alice"]);
  });
});

test("overlapping view renames wait for the acknowledged revision", async ({ page, request }) => {
  const { token } = await registerUser(request, "view-ordered");
  const client = createClient(request, token);
  await gotoWithSession(page, token);
  const wsID = await waitForWorkspace(page);
  const table = await client.ws(wsID).createTable("0", { title: "Ordered views", properties: [] });
  const created = await client.ws(wsID).createView(table.id, { name: "Initial", type: "table" });
  await client.ws(wsID).createView(table.id, { name: "Other", type: "table" });
  await page.goto(nodeUrl(wsID, undefined, table.id));
  await expect(page.getByRole("button", { name: "Initial", exact: true })).toBeVisible();

  let release!: () => void;
  let entered!: () => void;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const first = new Promise<void>((resolve) => {
    entered = resolve;
  });
  let writes = 0;
  await page.route(`**/nodes/${table.id}/views/${created.view.id}`, async (route) => {
    writes++;
    if (writes === 1) {
      entered();
      await gate;
    }
    await route.continue();
  });
  async function rename(from: string, to: string) {
    await page.getByRole("button", { name: from, exact: true }).click({ button: "right" });
    await page.getByRole("menuitem", { name: "Rename view" }).click();
    const input = page.locator("input").filter({ visible: true }).last();
    await input.fill(to);
    await input.press("Enter");
  }
  await rename("Initial", "First");
  await first;
  await page.getByRole("button", { name: "First", exact: true }).click({ button: "right" });
  await page.getByRole("menuitem", { name: "Rename view" }).click();
  const editing = page.locator("input").filter({ visible: true }).last();
  await editing.fill("Second");
  expect(writes).toBe(1);
  const acknowledged = page.waitForResponse(
    (response) => response.url().endsWith(`/views/${created.view.id}`) && response.request().method() === "POST",
  );
  release();
  await acknowledged;
  await page.evaluate(
    () => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))),
  );
  await expect
    .poll(async () => (await client.ws(wsID).getNode(table.id)).views?.find((v) => v.id === created.view.id)?.name)
    .toBe("First");
  await expect(editing).toBeFocused();
  await expect(editing).toHaveValue("Second");
  await editing.press("Enter");
  await expect
    .poll(async () => (await client.ws(wsID).getNode(table.id)).views?.find((v) => v.id === created.view.id)?.name)
    .toBe("Second");
  expect(writes).toBe(2);

  const current = (await client.ws(wsID).getNode(table.id)).views?.find((v) => v.id === created.view.id);
  expect(current).toBeDefined();
  await client.ws(wsID).updateView(table.id, created.view.id, {
    expected_revision: current!.revision,
    name: "Remote",
  });
  await rename("Second", "Local");
  const alert = page.getByRole("alert").filter({ hasText: "This view could not be saved" });
  await expect(alert).toBeVisible();
  await rename("Other", "Other saved");
  await expect
    .poll(async () => (await client.ws(wsID).getNode(table.id)).views?.find((v) => v.name === "Other saved")?.name)
    .toBe("Other saved");
  await expect(alert).toBeVisible();
  await page.setViewportSize({ width: 320, height: 640 });
  const bounds = await alert.boundingBox();
  expect(bounds).not.toBeNull();
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(320);
  let releaseRecovery!: () => void;
  let recoveryEntered!: () => void;
  const recoveryGate = new Promise<void>((resolve) => {
    releaseRecovery = resolve;
  });
  const recoveryStarted = new Promise<void>((resolve) => {
    recoveryEntered = resolve;
  });
  let recoveryReads = 0;
  await page.route(`**/nodes/${table.id}`, async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    recoveryReads++;
    const response = await route.fetch();
    recoveryEntered();
    await recoveryGate;
    await route.fulfill({ response });
  });
  const discard = alert.getByRole("button", { name: "Discard changes and reload" });
  await discard.click();
  await recoveryStarted;
  await discard.click();
  expect(recoveryReads).toBe(1);
  releaseRecovery();
  await expect(alert).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Remote", exact: true })).toBeVisible();
  await rename("Remote", "Recovered");
  await expect
    .poll(async () => (await client.ws(wsID).getNode(table.id)).views?.find((v) => v.id === created.view.id)?.name)
    .toBe("Recovered");
  expect(recoveryReads).toBe(1);
});

for (const preference of ["zoom", "default font"] as const) {
  test(`view conflict recovery fits with increased ${preference}`, async ({ browser, request, baseURL }, testInfo) => {
    const { mkdir, writeFile } = await import("node:fs/promises");
    const path = await import("node:path");
    const extension = testInfo.outputPath("view-zoom-extension");
    await mkdir(extension, { recursive: true });
    await writeFile(
      path.join(extension, "manifest.json"),
      JSON.stringify({
        manifest_version: 3,
        name: "View accessibility",
        permissions: ["fontSettings"],
        version: "1.0",
        background: { service_worker: "background.js" },
      }),
    );
    await writeFile(path.join(extension, "background.js"), "chrome.runtime.onInstalled.addListener(() => {});");
    const context = await browser.browserType().launchPersistentContext(testInfo.outputPath("view-zoom-profile"), {
      channel: "chromium",
      headless: true,
      baseURL,
      viewport: { width: 640, height: 900 },
      args: [`--disable-extensions-except=${extension}`, `--load-extension=${extension}`],
    });
    try {
      const worker = context.serviceWorkers()[0] ?? (await context.waitForEvent("serviceworker"));
      const page = context.pages()[0];
      if (!page) throw new Error("Missing browser page");
      await page.goto("/privacy");
      if (preference === "zoom") {
        await worker.evaluate("chrome.tabs.setZoom(2)");
        await expect.poll(() => page.evaluate(() => innerWidth)).toBe(320);
      } else {
        await worker.evaluate("chrome.fontSettings.setDefaultFontSize({ pixelSize: 24 })");
        await page.reload();
        await expect.poll(() => page.evaluate(() => getComputedStyle(document.documentElement).fontSize)).toBe("24px");
      }
      const { token } = await registerUser(request, "view-zoom");
      const client = createClient(request, token);
      await gotoWithSession(page, token);
      const wsID = await waitForWorkspace(page);
      const table = await client.ws(wsID).createTable("0", { title: "Zoom", properties: [] });
      const created = await client.ws(wsID).createView(table.id, { name: "Initial", type: "table" });
      await page.goto(nodeUrl(wsID, undefined, table.id));
      const tab = page.getByRole("button", { name: "Initial", exact: true });
      await expect(tab).toBeVisible();
      await client
        .ws(wsID)
        .updateView(table.id, created.view.id, { expected_revision: created.view.revision, name: "Remote" });
      await tab.click({ button: "right" });
      await page.getByRole("menuitem", { name: "Rename view" }).click();
      const input = page.locator("input").filter({ visible: true }).last();
      await input.fill("Local");
      await input.press("Enter");
      const alert = page.getByRole("alert").filter({ hasText: "This view could not be saved" });
      await expect(alert).toBeVisible();
      const dimensions = await alert.evaluate((element) => ({
        right: element.getBoundingClientRect().right,
        scrollWidth: element.scrollWidth,
        clientWidth: element.clientWidth,
      }));
      expect(dimensions.right).toBeLessThanOrEqual(preference === "zoom" ? 320 : 640);
      expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth);
      await expect(alert.getByRole("button", { name: "Discard changes and reload" })).toBeVisible();
    } finally {
      await context.close();
    }
  });
}
