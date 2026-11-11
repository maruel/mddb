// E2E tests for table creation, record CRUD, view modes, and sort UI.

import type { Page, APIRequestContext } from "@playwright/test";
import { test, expect, registerUser, waitForWorkspace, createClient, gotoWithSession } from "./helpers";
import type { DataRecordResponse, Property } from "../sdk/mddb/ts/v1/types.gen";
import { gate as requestGate } from "./gomode-fixtures";

// Helper: create a table with records and navigate to it.
async function setupTable(
  page: Page,
  request: APIRequestContext,
  prefix: string,
  properties: Property[],
  records: Record<string, unknown>[],
) {
  const { token } = await registerUser(request, prefix);
  await gotoWithSession(page, token);
  const wsID = await waitForWorkspace(page);
  const client = createClient(request, token);

  const tableData = await client.ws(wsID).createTable("0", {
    title: `${prefix} Table`,
    properties,
  });

  for (const data of records) {
    await client.ws(wsID).createRecord(tableData.id, { data });
  }

  await page.reload();
  await waitForWorkspace(page);
  await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();
  await expect(page.locator("table")).toBeVisible({ timeout: 5000 });

  return { token, wsID, client, tableId: tableData.id };
}

// Helper: extract the order of known names from table rows.
async function getRowOrder(page: Page, names: string[]): Promise<string[]> {
  const rows = page.locator("table tbody tr");
  const texts = await rows.allTextContents();
  return texts.map((t) => names.find((n) => t.includes(n)) ?? "").filter(Boolean);
}

test.describe("Table Creation and Basic Operations", () => {
  for (const outcome of ["success", "failure", "departure"] as const) {
    test(`schema acknowledgement preserves ownership after ${outcome}`, async ({ page, request }) => {
      const { client, wsID, tableId } = await setupTable(
        page,
        request,
        "schema-owner",
        [{ name: "Name", type: "text" }],
        [],
      );
      const ws = client.ws(wsID);
      const other = await ws.createPage("0", { title: "Other document", content: "Other body" });
      const requested = requestGate();
      const release = requestGate();
      const completed = requestGate();
      let held = false;
      await page.route(`**/nodes/${tableId}/table`, async (route) => {
        if (route.request().method() !== "POST" || held) return route.continue();
        held = true;
        const response = outcome === "failure" ? null : await route.fetch();
        requested.resolve();
        await release.promise;
        if (response) await route.fulfill({ response });
        else
          await route.fulfill({
            status: 409,
            json: { error: { code: "CONFLICT", message: "Schema acknowledgement refused" } },
          });
        completed.resolve();
      });
      await page.getByTitle("Add Column", { exact: true }).click();
      await page.getByPlaceholder("Column Name", { exact: true }).fill("Acknowledged");
      await page.getByRole("button", { name: "Confirm", exact: true }).click();
      await requested.promise;
      const title = page.getByRole("textbox", { name: "Title", exact: true });
      if (outcome === "departure") {
        await page.getByRole("treeitem", { name: "Other document", exact: true }).click();
        await expect(title).toHaveValue("Other document");
      } else {
        await page.clock.install();
        await page.clock.pauseAt(new Date());
        await title.fill("Private title during schema write");
      }
      release.resolve();
      await completed.promise;
      if (outcome === "success") {
        await expect(page.locator("th").getByText("Acknowledged", { exact: true })).toBeVisible();
        await expect(title).toHaveValue("Private title during schema write");
        // Return the title draft to its server value before another schema write.
        await title.fill("schema-owner Table");
        const acknowledged = await ws.getTable(tableId);
        const next = page.waitForRequest(
          (outgoing) => outgoing.method() === "POST" && outgoing.url().endsWith(`/nodes/${tableId}/table`),
        );
        await page.getByTitle("Add Column", { exact: true }).click();
        await page.getByPlaceholder("Column Name", { exact: true }).fill("Next column");
        await page.getByRole("button", { name: "Confirm", exact: true }).click();
        expect((await next).postDataJSON()).toMatchObject({ expected_schema_revision: acknowledged.schema_revision });
        await expect(page.locator("th").getByText("Next column", { exact: true })).toBeVisible();
      } else if (outcome === "failure") {
        await expect(page.getByRole("alert")).toContainText("Schema acknowledgement refused");
        await expect(title).toHaveValue("Private title during schema write");
        await expect(page.locator("th").getByText("Acknowledged", { exact: true })).toHaveCount(0);
      } else {
        await expect(title).toHaveValue("Other document");
        await expect(page.getByRole("alert")).toHaveCount(0);
        await expect(page.locator("th").getByText("Acknowledged", { exact: true })).toHaveCount(0);
        expect((await ws.getTable(tableId)).properties?.some((property) => property.name === "Acknowledged")).toBe(
          true,
        );
        expect((await ws.getNode(other.id)).title).toBe("Other document");
      }
      if (outcome !== "departure") await page.clock.resume();
    });
  }

  test.screenshot("create a table with properties and view it", async ({ page, request, takeScreenshot }) => {
    const { token } = await registerUser(request, "table-create");
    await gotoWithSession(page, token);

    const wsID = await waitForWorkspace(page);
    const client = createClient(request, token);
    const tableData = await client.ws(wsID).createTable("0", {
      title: "Test Table",
      properties: [
        { name: "Name", type: "text", required: true },
        {
          name: "Status",
          type: "select",
          options: [
            { id: "todo", name: "To Do" },
            { id: "done", name: "Done" },
          ],
        },
        { name: "Priority", type: "number" },
        { name: "Due Date", type: "date" },
      ],
    });

    await page.reload();
    await waitForWorkspace(page);
    await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();

    const tableElement = page.locator("table");
    await expect(tableElement).toBeVisible({ timeout: 5000 });

    const tableHeaders = page.locator("th");
    // Use exact:false since required columns append '*' to the name
    await expect(tableHeaders.getByText("Name", { exact: false })).toBeVisible();
    await expect(tableHeaders.getByText("Status", { exact: true })).toBeVisible();
    await expect(tableHeaders.getByText("Priority", { exact: true })).toBeVisible();
    await expect(tableHeaders.getByText("Due Date", { exact: true })).toBeVisible();

    await takeScreenshot("table-view");
  });

  test("add and edit records in table", async ({ page, request }) => {
    const { client, wsID, tableId } = await setupTable(
      page,
      request,
      "table-records",
      [
        { name: "Name", type: "text" },
        { name: "Value", type: "number" },
      ],
      [
        { Name: "Item 1", Value: 100 },
        { Name: "Item 2", Value: 200 },
      ],
    );

    await expect(page.getByText("Item 1", { exact: true })).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("200", { exact: true })).toBeVisible();

    // Click to edit
    await page.locator("td").getByText("Item 1", { exact: true }).click();
    const editInput = page.locator('table td input[type="text"]').first();
    await expect(editInput).toBeVisible({ timeout: 5000 });
    await editInput.fill("Edited Item 1");
    await editInput.press("Enter");

    // Verify via API
    const listParams = { ViewID: "", Filters: "", Sorts: "", Offset: 0, Limit: 100 };
    await expect(async () => {
      const data = await client.ws(wsID).listRecords(tableId, listParams);
      const edited = data.records?.find((r: DataRecordResponse) => (r.data?.Name as string) === "Edited Item 1");
      expect(edited).toBeTruthy();
    }).toPass({ timeout: 5000 });
  });

  test("preserves an attempted record edit when another client saves first", async ({ page, request }) => {
    const { client, wsID, tableId } = await setupTable(
      page,
      request,
      "record-conflict",
      [{ name: "Name", type: "text" }],
      [{ Name: "Original" }],
    );
    const ws = client.ws(wsID);
    const list = await ws.listRecords(tableId, { ViewID: "", Filters: "", Sorts: "", Offset: 0, Limit: 100 });
    const record = list.records![0]!;
    const schema = await ws.getNode(tableId);
    expect(schema.schema_revision).toBeTruthy();

    let release!: () => void;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route(`**/nodes/${tableId}/table/records/${record.id}`, async (route) => {
      if (route.request().method() !== "POST") return route.continue();
      await held;
      await route.continue();
    });
    await page.locator("td").getByText("Original", { exact: true }).click();
    const input = page.locator('table td input[type="text"]').first();
    await input.fill("My edit");
    const pending = page.waitForRequest(
      (req) => req.method() === "POST" && req.url().endsWith(`/records/${record.id}`),
    );
    await input.press("Enter");
    await pending;
    await ws.updateRecord(tableId, record.id, {
      data: { Name: "Other edit" },
      expected_revision: record.revision,
      expected_schema_revision: schema.schema_revision!,
    });
    release();
    await expect(
      page.getByText(
        "This record could not be saved. Review the current record before choosing whether to replace it.",
      ),
    ).toBeVisible();
    await expect(page.locator("pre").filter({ hasText: "My edit" })).toBeVisible();
    expect((await ws.getRecord(tableId, record.id)).data).toEqual({ Name: "Other edit" });
  });

  test("keeps concurrent rejected drafts through navigation and recovers only against reviewed revisions", async ({
    page,
    request,
  }) => {
    const { client, wsID, tableId } = await setupTable(
      page,
      request,
      "draft-navigation",
      [{ name: "Name", type: "text" }],
      [{ Name: "First" }, { Name: "Second" }],
    );
    const ws = client.ws(wsID);
    const other = await ws.createPage("0", { title: "Other page", content: "" });
    await page.reload();
    await waitForWorkspace(page);
    await page.locator(`[data-testid="sidebar-node-${tableId}"]`).click();
    const records =
      (await ws.listRecords(tableId, { ViewID: "", Filters: "", Sorts: "", Offset: 0, Limit: 100 })).records ?? [];
    const first = records.find((record) => record.data?.Name === "First");
    const second = records.find((record) => record.data?.Name === "Second");
    if (!first || !second) throw new Error("Missing test records");
    const schema = await ws.getNode(tableId);
    if (!schema.schema_revision) throw new Error("Missing schema revision");
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route(`**/nodes/${tableId}/table/records/*`, async (route) => {
      if (route.request().method() === "POST") await gate;
      await route.continue();
    });
    async function edit(name: string, value: string, id: string) {
      await page.locator("td").getByText(name, { exact: true }).click();
      const input = page.locator('table td input[type="text"]').first();
      await input.fill(value);
      const sent = page.waitForRequest((req) => req.method() === "POST" && req.url().endsWith(`/records/${id}`));
      await input.press("Enter");
      await sent;
    }
    const draftText = `Draft one ${"long text ".repeat(180)}`;
    await edit("First", draftText, first.id);
    await page.locator("td").getByText("Second", { exact: true }).click();
    const queuedInput = page.locator('table td input[type="text"]').first();
    await queuedInput.fill("Draft two");
    await queuedInput.press("Enter");
    await ws.updateRecord(tableId, first.id, {
      data: { Name: "Remote one" },
      expected_revision: first.revision,
      expected_schema_revision: schema.schema_revision,
    });
    await ws.updateRecord(tableId, second.id, {
      data: { Name: "Remote two" },
      expected_revision: second.revision,
      expected_schema_revision: schema.schema_revision,
    });
    await page.locator(`[data-testid="sidebar-node-${other.id}"]`).click();
    await expect(page).toHaveURL(new RegExp(other.id));
    await page.locator('aside [class*="workspaceHeader"]').hover();
    await page.getByTestId("workspace-settings-button").click();
    await expect(page).toHaveURL(/\/settings\/workspace\//);
    const response = page.waitForResponse(
      (res) => res.request().method() === "POST" && res.url().endsWith(`/records/${first.id}`),
    );
    release();
    await response;
    await page.unroute(`**/nodes/${tableId}/table/records/*`);
    await page.getByRole("button", { name: /Back/ }).click();
    await waitForWorkspace(page);
    await page.locator(`[data-testid="sidebar-node-${tableId}"]`).click();
    const drafts = page.getByRole("alert").filter({ hasText: "This record could not be saved" });
    await expect(drafts).toHaveCount(2);
    const draft = drafts.filter({ hasText: "Draft one" });
    await expect(drafts.filter({ hasText: "Draft two" })).toBeVisible();
    const originalText = draft.locator("pre").filter({ hasText: "Draft one" });
    await originalText.evaluate((el) => {
      el.scrollTop = 50;
    });
    expect(await originalText.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);
    let releaseReview!: () => void;
    let reviewEntered!: () => void;
    const reviewGate = new Promise<void>((resolve) => {
      releaseReview = resolve;
    });
    const reviewStarted = new Promise<void>((resolve) => {
      reviewEntered = resolve;
    });
    await page.route(`**/nodes/${tableId}/table/records/${first.id}`, async (route) => {
      if (route.request().method() === "GET") {
        reviewEntered();
        await reviewGate;
      }
      await route.continue();
    });
    const review = draft.getByRole("button", { name: "Review current record" });
    await review.focus();
    await page.keyboard.press("Enter");
    await reviewStarted;
    await expect(review).toBeFocused();
    await expect(review).toHaveAttribute("aria-disabled", "true");
    releaseReview();
    await expect(draft.locator("pre").filter({ hasText: "Remote one" })).toBeVisible();
    await expect(review).toBeFocused();
    expect(await originalText.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);
    await page.unroute(`**/nodes/${tableId}/table/records/${first.id}`);
    const remote = await ws.getRecord(tableId, first.id);
    await ws.updateRecord(tableId, first.id, {
      data: { Name: "Newer remote" },
      expected_revision: remote.revision,
      expected_schema_revision: schema.schema_revision,
    });
    const replace = draft.getByRole("button", { name: "Replace with my changes" });
    await replace.focus();
    await page.keyboard.press("Enter");
    await expect(draft.getByText("A conflict occurred with the current state")).toBeVisible();
    await expect(review).toBeFocused();
    expect((await ws.getRecord(tableId, first.id)).data).toEqual({ Name: "Newer remote" });
    await draft.getByRole("button", { name: "Review current record" }).click();
    await expect(draft.locator("pre").filter({ hasText: "Newer remote" })).toBeVisible();
    let releaseSave!: () => void;
    let saveEntered!: () => void;
    const saveGate = new Promise<void>((resolve) => {
      releaseSave = resolve;
    });
    const saveStarted = new Promise<void>((resolve) => {
      saveEntered = resolve;
    });
    await page.route(`**/nodes/${tableId}/table/records/${first.id}`, async (route) => {
      if (route.request().method() === "POST") {
        saveEntered();
        await saveGate;
      }
      await route.continue();
    });
    const recovered = draft.getByRole("button", { name: "Replace with my changes" });
    await recovered.focus();
    await page.keyboard.press("Enter");
    await saveStarted;
    await expect(recovered).toBeFocused();
    await expect(recovered).toHaveAttribute("aria-disabled", "true");
    expect(await originalText.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);
    releaseSave();
    await expect(drafts).toHaveCount(1);
    expect((await ws.getRecord(tableId, first.id)).data).toEqual({ Name: draftText });
    const remaining = drafts.filter({ hasText: "Draft two" });
    await expect(remaining).toBeVisible();
    await expect(remaining.getByRole("button", { name: "Review current record" })).toBeFocused();
    const discard = remaining.getByRole("button", { name: "Discard draft" });
    await discard.focus();
    await page.keyboard.press("Enter");
    await expect(drafts).toHaveCount(0);
    await expect(page.locator("[data-view-focus-target] button").first()).toBeFocused();
  });

  test("record edits undo and redo in order and reject intervening remote changes", async ({ page, request }) => {
    const { client, wsID, tableId } = await setupTable(
      page,
      request,
      "record-history",
      [{ name: "Name", type: "text" }],
      [{ Name: "Original" }],
    );
    const ws = client.ws(wsID);
    const record = (await ws.listRecords(tableId, { ViewID: "", Filters: "", Sorts: "", Offset: 0, Limit: 100 }))
      .records?.[0];
    const schema = await ws.getNode(tableId);
    if (!record || !schema.schema_revision) throw new Error("Missing record or schema");
    async function edit(from: string, to: string) {
      await page.locator("td").getByText(from, { exact: true }).click();
      const input = page.locator('table td input[type="text"]').first();
      await input.fill(to);
      const response = page.waitForResponse(
        (res) => res.request().method() === "POST" && res.url().endsWith(`/records/${record!.id}`),
      );
      await input.press("Enter");
      expect((await response).status()).toBe(200);
      await expect(page.locator("td").getByText(to, { exact: true })).toBeVisible();
      await page.getByRole("button", { name: "All", exact: true }).click();
    }
    async function history(redo: boolean, status: number) {
      const response = page.waitForResponse(
        (res) => res.request().method() === "POST" && res.url().endsWith(`/records/${record!.id}`),
      );
      await page.keyboard.press(redo ? "Control+Shift+z" : "Control+z");
      expect((await response).status()).toBe(status);
    }
    async function name(value: string) {
      await expect.poll(async () => (await ws.getRecord(tableId, record!.id)).data?.Name).toBe(value);
      await expect(page.locator("td").getByText(value, { exact: true })).toBeVisible();
    }
    await edit("Original", "First");
    await edit("First", "Second");
    await history(false, 200);
    await name("First");
    await history(false, 200);
    await name("Original");
    await history(true, 200);
    await name("First");
    await history(true, 200);
    await name("Second");
    await history(false, 200);
    await name("First");
    const current = await ws.getRecord(tableId, record.id);
    await ws.updateRecord(tableId, record.id, {
      data: { Name: "External" },
      expected_revision: current.revision,
      expected_schema_revision: schema.schema_revision,
    });
    await history(true, 409);
    expect((await ws.getRecord(tableId, record.id)).data).toEqual({ Name: "External" });
    await expect(page.getByTestId("workspace-feedback")).toBeVisible();
    await history(false, 409);
    expect((await ws.getRecord(tableId, record.id)).data).toEqual({ Name: "External" });
  });

  test("an edit admitted during a pending undo remains the newest undo step", async ({ page, request }) => {
    const { client, wsID, tableId } = await setupTable(
      page,
      request,
      "history-overlap",
      [{ name: "Name", type: "text" }],
      [{ Name: "Original" }],
    );
    const ws = client.ws(wsID);
    const record = (await ws.listRecords(tableId, { ViewID: "", Filters: "", Sorts: "", Offset: 0, Limit: 100 }))
      .records?.[0];
    if (!record) throw new Error("Missing record");
    async function edit(from: string, to: string) {
      await page.locator("td").getByText(from, { exact: true }).click();
      const input = page.locator('table td input[type="text"]').first();
      await input.fill(to);
      const done = page.waitForResponse(
        (res) => res.request().method() === "POST" && res.url().endsWith(`/records/${record!.id}`),
      );
      await input.press("Enter");
      expect((await done).status()).toBe(200);
      await expect(page.locator("td").getByText(to, { exact: true })).toBeVisible();
    }
    await edit("Original", "First");
    await page.getByRole("button", { name: "All", exact: true }).click();
    let release!: () => void;
    let entered!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const pending = new Promise<void>((resolve) => {
      entered = resolve;
    });
    await page.route(`**/nodes/${tableId}/table/records/${record.id}`, async (route) => {
      if (route.request().method() === "POST" && route.request().postDataJSON()?.data?.Name === "Original") {
        entered();
        await gate;
      }
      await route.continue();
    });
    await page.keyboard.press("Control+z");
    await pending;
    await page.locator("td").getByText("First", { exact: true }).click();
    const pendingInput = page.locator('table td input[type="text"]').first();
    await pendingInput.fill("Second");
    await pendingInput.press("Enter");
    const undoResponse = page.waitForResponse(
      (res) =>
        res.request().method() === "POST" &&
        res.url().endsWith(`/records/${record.id}`) &&
        res.request().postDataJSON()?.data?.Name === "Original",
    );
    release();
    expect((await undoResponse).status()).toBe(200);
    await expect.poll(async () => (await ws.getRecord(tableId, record.id)).data?.Name).toBe("Second");
    await page.unroute(`**/nodes/${tableId}/table/records/${record.id}`);
    await page.getByRole("button", { name: "All", exact: true }).click();
    const latestUndo = page.waitForResponse(
      (res) => res.request().method() === "POST" && res.url().endsWith(`/records/${record.id}`),
    );
    await page.keyboard.press("Control+z");
    expect((await latestUndo).status()).toBe(200);
    expect((await ws.getRecord(tableId, record.id)).data).toEqual({ Name: "Original" });
  });

  for (const scenario of ["remote edit", "schema change", "navigation"] as const) {
    test(`queued history edit retains its provenance through ${scenario}`, async ({ page, request }) => {
      const { client, wsID, tableId } = await setupTable(
        page,
        request,
        `queued-${scenario.replace(" ", "-")}`,
        [{ name: "Name", type: "text" }],
        [{ Name: "Original" }],
      );
      const ws = client.ws(wsID);
      const record = (await ws.listRecords(tableId, { ViewID: "", Filters: "", Sorts: "", Offset: 0, Limit: 100 }))
        .records?.[0];
      const schema = await ws.getNode(tableId);
      if (!record || !schema.schema_revision) throw new Error("Missing record or schema");
      const other = await ws.createPage("0", { title: "Queued destination", content: "" });
      await page.reload();
      await waitForWorkspace(page);
      await page.locator(`[data-testid="sidebar-node-${tableId}"]`).click();
      await page.locator("td").getByText("Original", { exact: true }).click();
      let input = page.locator('table td input[type="text"]').first();
      await input.fill("First");
      await input.press("Enter");
      await expect(page.locator("td").getByText("First", { exact: true })).toBeVisible();
      await page.getByRole("button", { name: "All", exact: true }).click();
      let release!: () => void;
      let entered!: () => void;
      let finished!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      const pending = new Promise<void>((resolve) => {
        entered = resolve;
      });
      const completed = new Promise<void>((resolve) => {
        finished = resolve;
      });
      await page.route(`**/nodes/${tableId}/table/records/${record.id}`, async (route) => {
        if (route.request().method() !== "POST" || route.request().postDataJSON()?.data?.Name !== "Original")
          return route.continue();
        const response = await route.fetch();
        entered();
        await gate;
        await route.fulfill({ response });
        finished();
      });
      await page.keyboard.press("Control+z");
      await pending;
      await page.locator("td").getByText("First", { exact: true }).click();
      input = page.locator('table td input[type="text"]').first();
      await input.fill("Queued draft");
      await input.press("Enter");
      if (scenario === "remote edit") {
        const current = await ws.getRecord(tableId, record.id);
        await ws.updateRecord(tableId, record.id, {
          data: { Name: "Remote" },
          expected_revision: current.revision,
          expected_schema_revision: schema.schema_revision,
        });
        await expect(page.locator("td").getByText("Remote", { exact: true })).toBeVisible();
      } else if (scenario === "schema change") {
        await ws.updateTable(tableId, {
          title: schema.title,
          expected_title: schema.title,
          properties: [
            { name: "Name", type: "text" },
            { name: "New column", type: "text" },
          ],
          expected_schema_revision: schema.schema_revision,
        });
      } else {
        await page.locator(`[data-testid="sidebar-node-${other.id}"]`).click();
      }
      release();
      await completed;
      await page.unroute(`**/nodes/${tableId}/table/records/${record.id}`);
      if (scenario === "navigation") await page.locator(`[data-testid="sidebar-node-${tableId}"]`).click();
      await expect(page.locator("pre").filter({ hasText: "Queued draft" })).toBeVisible();
      expect((await ws.getRecord(tableId, record.id)).data?.Name).toBe(
        scenario === "remote edit" ? "Remote" : "Original",
      );
    });
  }

  test("undo publishes its acknowledged record while its refresh is held", async ({ page, request }) => {
    const { client, wsID, tableId } = await setupTable(
      page,
      request,
      "undo-visible-state",
      [{ name: "Name", type: "text" }],
      [{ Name: "Original" }],
    );
    const ws = client.ws(wsID);
    const record = (await ws.listRecords(tableId, { ViewID: "", Filters: "", Sorts: "", Offset: 0, Limit: 100 }))
      .records?.[0];
    if (!record) throw new Error("Missing record");
    async function edit(from: string, to: string) {
      await page.locator("td").getByText(from, { exact: true }).click();
      const input = page.locator('table td input[type="text"]').first();
      await input.fill(to);
      await input.press("Enter");
      await expect(page.locator("td").getByText(to, { exact: true })).toBeVisible();
    }
    await edit("Original", "First");
    await page.getByRole("button", { name: "All", exact: true }).click();
    let release!: () => void;
    let entered!: () => void;
    let finished!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const pending = new Promise<void>((resolve) => {
      entered = resolve;
    });
    const completed = new Promise<void>((resolve) => {
      finished = resolve;
    });
    await page.route(
      `**/nodes/${tableId}/table/records?**`,
      async (route) => {
        const response = await route.fetch();
        entered();
        await gate;
        await route.fulfill({ response });
        finished();
      },
      { times: 1 },
    );
    await page.keyboard.press("Control+z");
    await pending;
    await expect(page.locator("td").getByText("Original", { exact: true })).toBeVisible();
    await edit("Original", "Second");
    await expect.poll(async () => (await ws.getRecord(tableId, record.id)).data?.Name).toBe("Second");
    release();
    await completed;
    await page.getByRole("button", { name: "All", exact: true }).click();
    await page.keyboard.press("Control+z");
    await expect.poll(async () => (await ws.getRecord(tableId, record.id)).data?.Name).toBe("Original");
    await expect(page.locator("td").getByText("Original", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "All", exact: true }).click();
    await page.keyboard.press("Control+Shift+z");
    await expect.poll(async () => (await ws.getRecord(tableId, record.id)).data?.Name).toBe("Second");
  });

  test("a newer edit remains above a recreated record while redo refresh is pending", async ({ page, request }) => {
    const { client, wsID, tableId } = await setupTable(
      page,
      request,
      "redo-refresh-order",
      [{ name: "Name", type: "text" }],
      [],
    );
    const ws = client.ws(wsID);
    const list = async () =>
      (await ws.listRecords(tableId, { ViewID: "", Filters: "", Sorts: "", Offset: 0, Limit: 100 })).records ?? [];
    await page.getByText(/add record/i).click();
    await expect(page.getByRole("button", { name: "Open", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "All", exact: true }).click();
    await page.keyboard.press("Control+z");
    await expect.poll(async () => (await list()).length).toBe(0);
    await expect(page.getByRole("button", { name: "Open", exact: true })).toHaveCount(0);
    let release!: () => void;
    let entered!: () => void;
    let finished!: () => void;
    const completed = new Promise<void>((resolve) => {
      finished = resolve;
    });
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const pending = new Promise<void>((resolve) => {
      entered = resolve;
    });
    let held = false;
    await page.route(`**/nodes/${tableId}/table/records?**`, async (route) => {
      if (!held && route.request().method() === "GET") {
        held = true;
        const response = await route.fetch();
        entered();
        await gate;
        await route.fulfill({ response });
        finished();
      } else await route.continue();
    });
    await page.keyboard.press("Control+Shift+z");
    await pending;
    const recreated = (await list())[0];
    if (!recreated) throw new Error("Missing recreated record");
    // Another creation refreshes the list while the redo's older refresh is held.
    await page.getByText(/add record/i).click();
    await expect(page.getByRole("button", { name: "Open", exact: true })).toHaveCount(2);
    const row = page.getByRole("button", { name: "Open", exact: true }).first().locator("xpath=ancestor::tr");
    await row.locator("td").nth(3).click();
    const input = page.locator('table td input[type="text"]').first();
    await input.fill("Newer");
    await input.press("Enter");
    await expect.poll(async () => (await list())[0]?.data?.Name).toBe("Newer");
    await expect(page.locator("td").getByText("Newer", { exact: true })).toBeVisible();
    release();
    await completed;
    await page.unroute(`**/nodes/${tableId}/table/records?**`);
    await page.getByRole("button", { name: "All", exact: true }).click();
    await page.keyboard.press("Control+z");
    await expect.poll(async () => (await list())[0]?.data?.Name ?? "").toBe("");
    expect((await list()).some((record) => record.id === recreated.id)).toBe(true);
  });

  test("queued field edits preserve acknowledged changes and undo effects", async ({ page, request }) => {
    const { client, wsID, tableId } = await setupTable(
      page,
      request,
      "field-intent",
      [
        { name: "Name", type: "text" },
        { name: "Status", type: "text" },
      ],
      [{ Name: "Original", Status: "Open" }],
    );
    const ws = client.ws(wsID);
    const record = (await ws.listRecords(tableId, { ViewID: "", Filters: "", Sorts: "", Offset: 0, Limit: 100 }))
      .records?.[0];
    if (!record) throw new Error("Missing record");
    async function edit(from: string, to: string) {
      await page.locator("td").getByText(from, { exact: true }).click();
      const input = page.locator('table td input[type="text"]').first();
      await input.fill(to);
      await input.press("Enter");
    }
    async function hold(name: string) {
      let release!: () => void;
      let entered!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      const pending = new Promise<void>((resolve) => {
        entered = resolve;
      });
      await page.route(`**/nodes/${tableId}/table/records/${record!.id}`, async (route) => {
        if (route.request().method() === "POST" && route.request().postDataJSON()?.data?.Name === name) {
          entered();
          await gate;
        }
        await route.continue();
      });
      return { release, pending };
    }
    const first = await hold("First");
    await edit("Original", "First");
    await first.pending;
    await edit("Open", "Done");
    first.release();
    await expect
      .poll(async () => (await ws.getRecord(tableId, record.id)).data)
      .toEqual({ Name: "First", Status: "Done" });
    await expect(page.locator("td").getByText("Done", { exact: true })).toBeVisible();
    await page.unroute(`**/nodes/${tableId}/table/records/${record.id}`);
    await page.getByRole("button", { name: "All", exact: true }).click();
    await page.keyboard.press("Control+z");
    await expect(page.locator("td").getByText("Open", { exact: true })).toBeVisible();
    const undo = await hold("Original");
    await page.getByRole("button", { name: "All", exact: true }).click();
    await page.keyboard.press("Control+z");
    await undo.pending;
    await edit("Open", "");
    undo.release();
    await expect
      .poll(async () => (await ws.getRecord(tableId, record.id)).data)
      .toEqual({ Name: "Original", Status: "" });
    await page.unroute(`**/nodes/${tableId}/table/records/${record.id}`);
  });

  test("ordinary update acknowledgement orders a pending undo and later edit", async ({ page, request }) => {
    const { client, wsID, tableId } = await setupTable(
      page,
      request,
      "update-ack-order",
      [{ name: "Name", type: "text" }],
      [{ Name: "Original" }, { Name: "Other" }],
    );
    const ws = client.ws(wsID);
    const list = async () =>
      (await ws.listRecords(tableId, { ViewID: "", Filters: "", Sorts: "", Offset: 0, Limit: 100 })).records ?? [];
    const original = (await list()).find((r) => r.data?.Name === "Original");
    const other = (await list()).find((r) => r.data?.Name === "Other");
    if (!original || !other) throw new Error("Missing records");
    async function edit(from: string, to: string) {
      await page.locator("td").getByText(from, { exact: true }).click();
      const input = page.locator('table td input[type="text"]').first();
      await input.fill(to);
      await input.press("Enter");
    }
    await edit("Other", "Other edited");
    await expect(page.locator("td").getByText("Other edited", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "All", exact: true }).click();
    let release!: () => void;
    let entered!: () => void;
    let finished!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const pending = new Promise<void>((resolve) => {
      entered = resolve;
    });
    const completed = new Promise<void>((resolve) => {
      finished = resolve;
    });
    await page.route(`**/nodes/${tableId}/table/records/${original.id}`, async (route) => {
      if (route.request().method() !== "POST" || route.request().postDataJSON()?.data?.Name !== "First")
        return route.continue();
      const response = await route.fetch();
      entered();
      await gate;
      await route.fulfill({ response });
      finished();
    });
    await edit("Original", "First");
    await pending;
    await ws.createRecord(tableId, { data: { Name: "Refresh" } });
    await expect(page.locator("td").getByText("First", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "All", exact: true }).click();
    await page.keyboard.press("Control+z");
    await edit("First", "Second");
    release();
    await completed;
    await page.unroute(`**/nodes/${tableId}/table/records/${original.id}`);
    await expect.poll(async () => (await ws.getRecord(tableId, original.id)).data?.Name).toBe("Second");
    await expect.poll(async () => (await ws.getRecord(tableId, other.id)).data?.Name).toBe("Other edited");
    await expect(page.locator("td").getByText("Second", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "All", exact: true }).click();
    await page.keyboard.press("Control+z");
    await expect.poll(async () => (await ws.getRecord(tableId, original.id)).data?.Name).toBe("Original");
  });

  test("an initial creation acknowledgement orders later edits", async ({ page, request }) => {
    const { client, wsID, tableId } = await setupTable(
      page,
      request,
      "create-ack-order",
      [{ name: "Name", type: "text" }],
      [],
    );
    const ws = client.ws(wsID);
    const list = async () =>
      (await ws.listRecords(tableId, { ViewID: "", Filters: "", Sorts: "", Offset: 0, Limit: 100 })).records ?? [];
    let release!: () => void;
    let entered!: () => void;
    let finished!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const pending = new Promise<void>((resolve) => {
      entered = resolve;
    });
    const completed = new Promise<void>((resolve) => {
      finished = resolve;
    });
    await page.route(
      `**/nodes/${tableId}/table/records/create`,
      async (route) => {
        const response = await route.fetch();
        entered();
        await gate;
        await route.fulfill({ response });
        finished();
      },
      { times: 1 },
    );
    await page.getByText(/add record/i).click();
    await pending;
    const created = (await list())[0];
    if (!created) throw new Error("Missing created record");
    await ws.createRecord(tableId, { data: { Name: "Other" } });
    await expect(page.getByRole("button", { name: "Open", exact: true })).toHaveCount(2);
    const row = page.getByRole("button", { name: "Open", exact: true }).first().locator("xpath=ancestor::tr");
    await row.locator("td").nth(3).click();
    const input = page.locator('table td input[type="text"]').first();
    await input.fill("Newer");
    await input.press("Enter");
    release();
    await completed;
    await expect.poll(async () => (await ws.getRecord(tableId, created.id)).data?.Name).toBe("Newer");
    await expect(page.locator("td").getByText("Newer", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "All", exact: true }).click();
    await page.keyboard.press("Control+z");
    await expect.poll(async () => (await ws.getRecord(tableId, created.id)).data?.Name ?? "").toBe("");
    expect((await list()).some((record) => record.id === created.id)).toBe(true);
  });

  test("a recreation acknowledgement owns edits to its visible record", async ({ page, request }) => {
    const { client, wsID, tableId } = await setupTable(
      page,
      request,
      "redo-response-order",
      [{ name: "Name", type: "text" }],
      [],
    );
    const ws = client.ws(wsID);
    const list = async () =>
      (await ws.listRecords(tableId, { ViewID: "", Filters: "", Sorts: "", Offset: 0, Limit: 100 })).records ?? [];
    await page.getByText(/add record/i).click();
    await expect(page.getByRole("button", { name: "Open", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "All", exact: true }).click();
    await page.keyboard.press("Control+z");
    await expect(page.getByRole("button", { name: "Open", exact: true })).toHaveCount(0);
    let release!: () => void;
    let entered!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const pending = new Promise<void>((resolve) => {
      entered = resolve;
    });
    let finished!: () => void;
    const completed = new Promise<void>((resolve) => {
      finished = resolve;
    });
    await page.route(`**/nodes/${tableId}/table/records/create`, async (route) => {
      const response = await route.fetch();
      entered();
      await gate;
      await route.fulfill({ response });
      finished();
    });
    await page.keyboard.press("Control+Shift+z");
    await pending;
    const recreated = (await list())[0];
    if (!recreated) throw new Error("Missing recreated record");
    // An external change prompts the normal subscription refresh while the create response is held.
    await ws.createRecord(tableId, { data: { Name: "Other" } });
    await expect(page.getByRole("button", { name: "Open", exact: true })).toHaveCount(2);
    const row = page.getByRole("button", { name: "Open", exact: true }).first().locator("xpath=ancestor::tr");
    await row.locator("td").nth(3).click();
    const input = page.locator('table td input[type="text"]').first();
    await input.fill("Newer");
    await input.press("Enter");
    release();
    await completed;
    await page.unroute(`**/nodes/${tableId}/table/records/create`);
    await expect.poll(async () => (await ws.getRecord(tableId, recreated.id)).data?.Name).toBe("Newer");
    await expect(page.locator("td").getByText("Newer", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "All", exact: true }).click();
    await page.keyboard.press("Control+z");
    await expect.poll(async () => (await ws.getRecord(tableId, recreated.id)).data?.Name ?? "").toBe("");
    expect((await list()).some((record) => record.id === recreated.id)).toBe(true);
  });

  test("record history follows recreated identities and rejects stale deletion redo", async ({ page, request }) => {
    const { client, wsID, tableId } = await setupTable(
      page,
      request,
      "record-identity",
      [{ name: "Name", type: "text" }],
      [],
    );
    const ws = client.ws(wsID);
    const list = async () =>
      (await ws.listRecords(tableId, { ViewID: "", Filters: "", Sorts: "", Offset: 0, Limit: 100 })).records ?? [];
    await page.getByText(/add record/i).click();
    await expect.poll(async () => (await list()).length).toBe(1);
    await expect(page.getByRole("button", { name: "Open", exact: true })).toBeVisible();
    const original = (await list())[0];
    if (!original) throw new Error("Missing created record");
    const row = page.getByRole("button", { name: "Open", exact: true }).locator("xpath=ancestor::tr");
    await row.locator("td").nth(3).click();
    const input = page.locator('table td input[type="text"]').first();
    await input.fill("First");
    const editResponse = page.waitForResponse(
      (res) => res.request().method() === "POST" && res.url().endsWith(`/records/${original.id}`),
    );
    await input.press("Enter");
    expect((await editResponse).status()).toBe(200);
    await expect(page.locator("td").getByText("First", { exact: true })).toBeVisible();
    async function history(redo: boolean, suffix: string, status: number) {
      await page.getByRole("button", { name: "All", exact: true }).click();
      const response = page.waitForResponse((res) => res.request().method() === "POST" && res.url().includes(suffix));
      await page.keyboard.press(redo ? "Control+Shift+z" : "Control+z");
      expect((await response).status()).toBe(status);
    }
    await history(false, `/records/${original.id}`, 200);
    await expect.poll(async () => (await list())[0]?.data?.Name ?? "").toBe("");
    await history(false, `/records/${original.id}/delete`, 200);
    await expect.poll(async () => (await list()).length).toBe(0);
    await history(true, "/records/create", 200);
    await expect.poll(async () => (await list()).length).toBe(1);
    const recreated = (await list())[0];
    if (!recreated || recreated.id === original.id) throw new Error("Record was not recreated");
    await history(true, `/records/${recreated.id}`, 200);
    await expect.poll(async () => (await list())[0]?.data?.Name).toBe("First");
    await expect(page.locator("td").getByText("First", { exact: true })).toBeVisible();
    page.once("dialog", (dialog) => void dialog.accept());
    const deleteResponse = page.waitForResponse(
      (res) => res.request().method() === "POST" && res.url().endsWith(`/records/${recreated.id}/delete`),
    );
    await page.locator("tr").filter({ hasText: "First" }).locator("button", { hasText: "✕" }).click();
    expect((await deleteResponse).status()).toBe(200);
    await expect.poll(async () => (await list()).length).toBe(0);
    await history(false, "/records/create", 200);
    await expect.poll(async () => (await list()).length).toBe(1);
    const restored = (await list())[0];
    if (!restored || restored.id === recreated.id) throw new Error("Deleted record was not recreated");
    await history(false, `/records/${restored.id}`, 200);
    await expect.poll(async () => (await list())[0]?.data?.Name ?? "").toBe("");
    await history(true, `/records/${restored.id}`, 200);
    await expect.poll(async () => (await list())[0]?.data?.Name).toBe("First");
    const latest = await ws.getRecord(tableId, restored.id);
    const schema = await ws.getNode(tableId);
    if (!schema.schema_revision) throw new Error("Missing schema revision");
    await ws.updateRecord(tableId, restored.id, {
      data: { Name: "Remote" },
      expected_revision: latest.revision,
      expected_schema_revision: schema.schema_revision,
    });
    await history(true, `/records/${restored.id}/delete`, 409);
    expect((await ws.getRecord(tableId, restored.id)).data).toEqual({ Name: "Remote" });
    await expect(page.getByTestId("workspace-feedback")).toBeVisible();
  });

  test("undoing record creation preserves a later remote edit", async ({ page, request }) => {
    const { client, wsID, tableId } = await setupTable(
      page,
      request,
      "record-create-undo",
      [{ name: "Name", type: "text" }],
      [],
    );
    const ws = client.ws(wsID);
    const list = () => ws.listRecords(tableId, { ViewID: "", Filters: "", Sorts: "", Offset: 0, Limit: 100 });
    await page.getByText(/add record/i).click();
    await expect.poll(async () => (await list()).records?.length).toBe(1);
    const record = (await list()).records?.[0];
    const schema = await ws.getNode(tableId);
    if (!record || !schema.schema_revision) throw new Error("Missing record or schema");
    await expect(page.getByRole("button", { name: "Open", exact: true })).toBeVisible();
    await ws.updateRecord(tableId, record.id, {
      data: { Name: "Remote edit" },
      expected_revision: record.revision,
      expected_schema_revision: schema.schema_revision,
    });
    const result = page.waitForResponse(
      (response) => response.request().method() === "POST" && response.url().endsWith(`/records/${record.id}/delete`),
    );
    await page.keyboard.press("Control+z");
    expect((await result).status()).toBe(409);
    expect((await ws.getRecord(tableId, record.id)).data).toEqual({ Name: "Remote edit" });
    await expect(page.getByTestId("workspace-feedback")).toBeVisible();
  });

  test("delete a record from table", async ({ page, request }) => {
    await setupTable(page, request, "table-delete", [{ name: "Name", type: "text" }], [{ Name: "Record To Delete" }]);

    await expect(page.getByText("Record To Delete", { exact: true })).toBeVisible({ timeout: 5000 });

    page.once("dialog", async (dialog) => await dialog.accept());
    const recordRow = page.locator("tr").filter({ hasText: "Record To Delete" });
    await recordRow.locator("button", { hasText: "✕" }).click();

    await expect(page.locator("td").filter({ hasText: "Record To Delete" })).not.toBeVisible({ timeout: 5000 });
  });

  test("add record via UI button", async ({ page, request }) => {
    const { client, wsID, tableId } = await setupTable(
      page,
      request,
      "table-add-ui",
      [{ name: "Name", type: "text" }],
      [],
    );

    // Empty table should show "+ Add Record"
    await expect(page.getByText(/add record/i)).toBeVisible({ timeout: 5000 });

    // Click to add
    await page.getByText(/add record/i).click();

    // Verify record was created via API
    const listParams = { ViewID: "", Filters: "", Sorts: "", Offset: 0, Limit: 100 };
    await expect(async () => {
      const data = await client.ws(wsID).listRecords(tableId, listParams);
      expect(data.records?.length).toBe(1);
    }).toPass({ timeout: 5000 });
  });
});

test.describe("Table View Modes", () => {
  test.screenshot("table has view tabs and add view dropdown", async ({ page, request, takeScreenshot }) => {
    await setupTable(
      page,
      request,
      "view-modes",
      [
        { name: "Name", type: "text" },
        {
          name: "Status",
          type: "select",
          options: [
            { id: "todo", name: "To Do" },
            { id: "progress", name: "In Progress" },
            { id: "done", name: "Done" },
          ],
        },
      ],
      [
        { Name: "Task 1", Status: "todo" },
        { Name: "Task 2", Status: "progress" },
      ],
    );

    await expect(page.locator("td").getByText("Task 1", { exact: true })).toBeVisible({ timeout: 5000 });

    // Default view tab active
    const defaultViewTab = page.locator("button").filter({ hasText: "All" });
    await expect(defaultViewTab).toBeVisible();
    await expect(defaultViewTab).toHaveClass(/active/i);
    await takeScreenshot("view-table");

    // Add view dropdown
    const addViewButton = page.locator('[data-testid="add-view-button"]');
    await addViewButton.click();
    const viewMenu = page.locator('[data-testid="view-type-menu"]');
    await expect(viewMenu).toBeVisible({ timeout: 3000 });
    await expect(page.locator('[data-testid="view-type-table"]')).toBeVisible();
    await expect(page.locator('[data-testid="view-type-gallery"]')).toBeVisible();
    await expect(page.locator('[data-testid="view-type-board"]')).toBeVisible();
    await takeScreenshot("view-dropdown");

    // Close by clicking outside
    await page.locator("body").click({ position: { x: 400, y: 400 } });
    await expect(viewMenu).not.toBeVisible({ timeout: 3000 });
  });
});

test.describe("Table Sort UI", () => {
  test("column header sort menu appears and closes", async ({ page, request }) => {
    await setupTable(page, request, "sort-ui", [{ name: "Name", type: "text" }], [{ Name: "Alice" }]);

    await expect(page.getByText("Alice", { exact: true })).toBeVisible({ timeout: 5000 });

    // Right-click column header to open context menu
    await page.locator("th").filter({ hasText: "Name" }).first().click({ button: "right" });
    await expect(page.locator('[data-testid="context-menu-sort-asc"]')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('[data-testid="context-menu-sort-desc"]')).toBeVisible();

    // Close with Escape
    await page.keyboard.press("Escape");
    await expect(page.locator('[data-testid="context-menu-sort-asc"]')).not.toBeVisible({ timeout: 3000 });
  });

  test("sort ascending via column header", async ({ page, request }) => {
    await setupTable(
      page,
      request,
      "sort-asc",
      [{ name: "Name", type: "text" }],
      [{ Name: "Zebra" }, { Name: "Apple" }, { Name: "Mango" }],
    );

    await expect(page.getByText("Zebra", { exact: true })).toBeVisible({ timeout: 5000 });

    // Sort ascending by clicking the column header once
    await page.locator("th").filter({ hasText: "Name" }).first().click();

    // Records should be: Apple, Mango, Zebra
    await expect(async () => {
      expect(await getRowOrder(page, ["Apple", "Mango", "Zebra"])).toEqual(["Apple", "Mango", "Zebra"]);
    }).toPass({ timeout: 5000 });

    // Sort indicator should appear on header
    await expect(page.locator('[data-testid="sort-indicator"]')).toBeVisible();
  });

  test("sort descending via column header", async ({ page, request }) => {
    await setupTable(
      page,
      request,
      "sort-desc",
      [{ name: "Name", type: "text" }],
      [{ Name: "Zebra" }, { Name: "Apple" }, { Name: "Mango" }],
    );

    await expect(page.getByText("Zebra", { exact: true })).toBeVisible({ timeout: 5000 });

    // Sort descending: click once (asc), then again (desc)
    await page.locator("th").filter({ hasText: "Name" }).first().click();
    await page.locator("th").filter({ hasText: "Name" }).first().click();

    await expect(async () => {
      expect(await getRowOrder(page, ["Apple", "Mango", "Zebra"])).toEqual(["Zebra", "Mango", "Apple"]);
    }).toPass({ timeout: 5000 });
  });

  test("remove sort clears ordering", async ({ page, request }) => {
    await setupTable(
      page,
      request,
      "sort-remove",
      [{ name: "Name", type: "text" }],
      [{ Name: "Zebra" }, { Name: "Apple" }],
    );

    await expect(page.getByText("Zebra", { exact: true })).toBeVisible({ timeout: 5000 });

    // Add ascending sort by clicking header once
    await page.locator("th").filter({ hasText: "Name" }).first().click();
    await expect(page.locator('[data-testid="sort-indicator"]')).toBeVisible({ timeout: 3000 });

    // Remove sort via chips bar
    await page.locator('[data-testid="sort-chip-Name"] button').click();

    // Sort indicator should be gone
    await expect(page.locator('[data-testid="sort-indicator"]')).not.toBeVisible({ timeout: 3000 });
  });

  test("multiple sorts apply compound ordering", async ({ page, request }) => {
    await setupTable(
      page,
      request,
      "sort-multi",
      [
        { name: "Color", type: "text" },
        { name: "Size", type: "number" },
      ],
      [
        { Color: "Red", Size: 3 },
        { Color: "Blue", Size: 1 },
        { Color: "Red", Size: 1 },
        { Color: "Blue", Size: 2 },
      ],
    );

    await expect(page.getByText("Red", { exact: true }).first()).toBeVisible({ timeout: 5000 });

    // Sort by Color ascending (click once)
    await page.locator("th").filter({ hasText: "Color" }).first().click();

    // Sort by Size ascending (additive, click once)
    await page.locator("th").filter({ hasText: "Size" }).first().click();

    // Both columns should show sort indicators
    await expect(page.locator('[data-testid="sort-indicator"]')).toHaveCount(2, { timeout: 3000 });

    // Order: Blue/1, Blue/2, Red/1, Red/3
    await expect(async () => {
      const rows = page.locator("table tbody tr").filter({ has: page.locator("td") });
      const count = await rows.count();
      const pairs: string[] = [];
      for (let i = 0; i < count; i++) {
        const cells = rows.nth(i).locator("td");
        const cellTexts = await cells.allTextContents();
        const color = cellTexts.find((c) => c === "Blue" || c === "Red");
        const size = cellTexts.find((c) => /^[123]$/.test(c.trim()));
        if (color && size) pairs.push(`${color}/${size.trim()}`);
      }
      expect(pairs).toEqual(["Blue/1", "Blue/2", "Red/1", "Red/3"]);
    }).toPass({ timeout: 5000 });
  });

  test("sort persists on saved view", async ({ page, request }) => {
    const { client, wsID, tableId } = await setupTable(
      page,
      request,
      "sort-persist",
      [{ name: "Name", type: "text" }],
      [{ Name: "Zebra" }, { Name: "Apple" }, { Name: "Mango" }],
    );

    await expect(page.getByText("Zebra", { exact: true })).toBeVisible({ timeout: 5000 });

    // Create a saved view via API
    const viewData = await client.ws(wsID).createView(tableId, {
      name: "Sorted",
      type: "table",
    });

    // Reload to pick up the new view
    await page.reload();
    await waitForWorkspace(page);
    await page.locator(`[data-testid="sidebar-node-${tableId}"]`).click();
    await expect(page.locator("table")).toBeVisible({ timeout: 5000 });

    // Switch to the Sorted view tab
    await page.locator("button").filter({ hasText: "Sorted" }).click();

    // Sort ascending by clicking the column header once
    await page.locator("th").filter({ hasText: "Name" }).first().click();

    // Wait for records to sort
    await expect(async () => {
      expect(await getRowOrder(page, ["Apple", "Mango", "Zebra"])).toEqual(["Apple", "Mango", "Zebra"]);
    }).toPass({ timeout: 5000 });

    // Verify sort was persisted via API
    await expect(async () => {
      const listParams = {
        ViewID: viewData.view.id,
        Filters: "",
        Sorts: "",
        Offset: 0,
        Limit: 100,
      };
      const data = await client.ws(wsID).listRecords(tableId, listParams);
      const names = data.records?.map((r: DataRecordResponse) => r.data?.Name);
      expect(names).toEqual(["Apple", "Mango", "Zebra"]);
    }).toPass({ timeout: 5000 });
  });
});

test.describe("Table Filter UI", () => {
  test("filter panel opens from column header menu", async ({ page, request }) => {
    await setupTable(
      page,
      request,
      "filter-open",
      [{ name: "Name", type: "text" }],
      [{ Name: "Alice" }, { Name: "Bob" }],
    );

    await page.locator("th").filter({ hasText: "Name" }).first().click({ button: "right" });
    await expect(page.locator('[data-testid="context-menu-filter-by"]')).toBeVisible({ timeout: 3000 });
    await page.locator('[data-testid="context-menu-filter-by"]').click();
    await expect(page.locator('[data-testid="filter-panel"]')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('[data-testid="filter-operator"]')).toBeVisible();
    await expect(page.locator('[data-testid="filter-apply"]')).toBeVisible();
  });

  test("filter by contains hides non-matching records", async ({ page, request }) => {
    await setupTable(
      page,
      request,
      "filter-contains",
      [{ name: "Name", type: "text" }],
      [{ Name: "Alice" }, { Name: "Bob" }, { Name: "Charlie" }],
    );

    await expect(page.getByText("Alice", { exact: true })).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("Bob", { exact: true })).toBeVisible();
    await expect(page.getByText("Charlie", { exact: true })).toBeVisible();

    // Open filter panel for Name column
    await page.locator("th").filter({ hasText: "Name" }).first().click({ button: "right" });
    await page.locator('[data-testid="context-menu-filter-by"]').click();
    await expect(page.locator('[data-testid="filter-panel"]')).toBeVisible({ timeout: 3000 });

    // Set operator to "contains" and value to "li"
    await page.locator('[data-testid="filter-operator"]').selectOption("contains");
    await page.locator('[data-testid="filter-value"]').fill("li");
    await page.locator('[data-testid="filter-apply"]').click();

    // Only Alice and Charlie contain "li"
    await expect(async () => {
      await expect(page.getByText("Alice", { exact: true })).toBeVisible();
      await expect(page.getByText("Charlie", { exact: true })).toBeVisible();
      await expect(page.getByText("Bob", { exact: true })).not.toBeVisible();
    }).toPass({ timeout: 5000 });
  });

  test("filter by equals shows only exact match", async ({ page, request }) => {
    await setupTable(
      page,
      request,
      "filter-equals",
      [{ name: "Name", type: "text" }],
      [{ Name: "Alice" }, { Name: "Bob" }, { Name: "Alice B" }],
    );

    await expect(page.getByText("Alice", { exact: true })).toBeVisible({ timeout: 5000 });

    await page.locator("th").filter({ hasText: "Name" }).first().click({ button: "right" });
    await page.locator('[data-testid="context-menu-filter-by"]').click();
    await expect(page.locator('[data-testid="filter-panel"]')).toBeVisible({ timeout: 3000 });

    await page.locator('[data-testid="filter-operator"]').selectOption("equals");
    await page.locator('[data-testid="filter-value"]').fill("Alice");
    await page.locator('[data-testid="filter-apply"]').click();

    await expect(async () => {
      const rows = page.locator("table tbody tr:not(.newRow)");
      const visible = await rows.filter({ hasText: "Alice" }).count();
      expect(visible).toBe(1);
      await expect(page.getByText("Bob", { exact: true })).not.toBeVisible();
    }).toPass({ timeout: 5000 });
  });

  test("remove filter restores all records", async ({ page, request }) => {
    await setupTable(
      page,
      request,
      "filter-remove",
      [{ name: "Name", type: "text" }],
      [{ Name: "Alice" }, { Name: "Bob" }],
    );

    await expect(page.getByText("Alice", { exact: true })).toBeVisible({ timeout: 5000 });

    // Apply a filter
    await page.locator("th").filter({ hasText: "Name" }).first().click({ button: "right" });
    await page.locator('[data-testid="context-menu-filter-by"]').click();
    await expect(page.locator('[data-testid="filter-panel"]')).toBeVisible({ timeout: 3000 });
    await page.locator('[data-testid="filter-operator"]').selectOption("equals");
    await page.locator('[data-testid="filter-value"]').fill("Alice");
    await page.locator('[data-testid="filter-apply"]').click();

    await expect(async () => {
      await expect(page.getByText("Bob", { exact: true })).not.toBeVisible();
    }).toPass({ timeout: 5000 });

    // Remove the filter
    await page.locator("th").filter({ hasText: "Name" }).first().click({ button: "right" });
    await page.locator('[data-testid="context-menu-filter-by"]').click();
    await expect(page.locator('[data-testid="filter-panel"]')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('[data-testid="filter-remove"]')).toBeVisible();
    await page.locator('[data-testid="filter-remove"]').click();

    await expect(async () => {
      await expect(page.getByText("Alice", { exact: true })).toBeVisible();
      await expect(page.getByText("Bob", { exact: true })).toBeVisible();
    }).toPass({ timeout: 5000 });
  });

  test("filter indicator appears in column header when filter is active", async ({ page, request }) => {
    await setupTable(
      page,
      request,
      "filter-indicator",
      [{ name: "Name", type: "text" }],
      [{ Name: "Alice" }, { Name: "Bob" }],
    );

    // No indicator initially
    await expect(page.locator('[data-testid="filter-indicator"]')).not.toBeVisible();

    // Apply a filter
    await page.locator("th").filter({ hasText: "Name" }).first().click({ button: "right" });
    await page.locator('[data-testid="context-menu-filter-by"]').click();
    await expect(page.locator('[data-testid="filter-panel"]')).toBeVisible({ timeout: 3000 });
    await page.locator('[data-testid="filter-value"]').fill("Alice");
    await page.locator('[data-testid="filter-apply"]').click();

    // Indicator appears
    await expect(page.locator('[data-testid="filter-indicator"]')).toBeVisible({ timeout: 3000 });
  });

  test("two-column filters compound (AND)", async ({ page, request }) => {
    await setupTable(
      page,
      request,
      "filter-compound",
      [
        { name: "Name", type: "text" },
        { name: "Age", type: "number" },
      ],
      [
        { Name: "Alice", Age: 30 },
        { Name: "Bob", Age: 25 },
        { Name: "Charlie", Age: 30 },
      ],
    );

    await expect(page.getByText("Alice", { exact: true })).toBeVisible({ timeout: 5000 });

    // Filter Name contains 'li'
    await page.locator("th").filter({ hasText: "Name" }).first().click({ button: "right" });
    await page.locator('[data-testid="context-menu-filter-by"]').click();
    await expect(page.locator('[data-testid="filter-panel"]')).toBeVisible({ timeout: 3000 });
    await page.locator('[data-testid="filter-operator"]').selectOption("contains");
    await page.locator('[data-testid="filter-value"]').fill("li");
    await page.locator('[data-testid="filter-apply"]').click();

    // Alice and Charlie pass; Bob is hidden
    await expect(async () => {
      await expect(page.getByText("Bob", { exact: true })).not.toBeVisible();
    }).toPass({ timeout: 5000 });

    // Filter Age equals 30
    await page.locator("th").filter({ hasText: "Age" }).first().click({ button: "right" });
    await page.locator('[data-testid="context-menu-filter-by"]').click();
    await expect(page.locator('[data-testid="filter-panel"]')).toBeVisible({ timeout: 3000 });
    await page.locator('[data-testid="filter-operator"]').selectOption("equals");
    await page.locator('[data-testid="filter-value"]').fill("30");
    await page.locator('[data-testid="filter-apply"]').click();

    // Only Alice and Charlie remain (both have li AND age=30)
    await expect(async () => {
      await expect(page.getByText("Alice", { exact: true })).toBeVisible();
      await expect(page.getByText("Charlie", { exact: true })).toBeVisible();
      await expect(page.getByText("Bob", { exact: true })).not.toBeVisible();
    }).toPass({ timeout: 5000 });
  });
});

test.describe("Table and Page Hybrid", () => {
  test("table node shows table view with records section", async ({ page, request }) => {
    const { token } = await registerUser(request, "hybrid-node");
    await gotoWithSession(page, token);

    const wsID = await waitForWorkspace(page);
    const client = createClient(request, token);
    const tableData = await client.ws(wsID).createTable("0", {
      title: "Table Only Node",
      properties: [{ name: "Item", type: "text" }],
    });

    await page.reload();
    await waitForWorkspace(page);
    await page.locator(`[data-testid="sidebar-node-${tableData.id}"]`).click();

    await expect(page.locator('input[placeholder*="Title"]')).toHaveValue("Table Only Node", { timeout: 5000 });
    await expect(page.locator("table")).toBeVisible({ timeout: 5000 });
    await expect(page.locator("button").filter({ hasText: "All" })).toBeVisible();
    await expect(page.getByTitle("New View")).toBeVisible();
    await expect(page.locator('textarea[placeholder*="markdown"]')).not.toBeVisible();
  });
});

for (const preference of ["default font", "zoom"] as const) {
  test(`German record draft review remains usable with ${preference} on a narrow screen`, async ({
    browser,
    request,
    baseURL,
  }, testInfo) => {
    const { mkdir, writeFile } = await import("node:fs/promises");
    const path = await import("node:path");
    const extension = testInfo.outputPath("record-font-extension");
    await mkdir(extension, { recursive: true });
    await writeFile(
      path.join(extension, "manifest.json"),
      JSON.stringify({
        manifest_version: 3,
        name: "Record font",
        version: "1.0",
        permissions: ["fontSettings"],
        background: { service_worker: "background.js" },
      }),
    );
    await writeFile(path.join(extension, "background.js"), "chrome.runtime.onInstalled.addListener(() => {});");
    const context = await browser.browserType().launchPersistentContext(testInfo.outputPath("record-font-profile"), {
      channel: "chromium",
      headless: true,
      baseURL,
      viewport: { width: 1280, height: 900 },
      args: [`--disable-extensions-except=${extension}`, `--load-extension=${extension}`],
    });
    try {
      const worker = context.serviceWorkers()[0] ?? (await context.waitForEvent("serviceworker"));
      const page = context.pages()[0];
      if (!page) throw new Error("Missing browser page");
      await page.goto("/privacy");
      if (preference === "default font") {
        await worker.evaluate("chrome.fontSettings.setDefaultFontSize({ pixelSize: 24 })");
        await page.reload();
        await expect.poll(() => page.evaluate(() => getComputedStyle(document.documentElement).fontSize)).toBe("24px");
      }
      const { client, wsID, tableId } = await setupTable(
        page,
        request,
        "draft-font",
        [{ name: "Name", type: "text" }],
        [{ Name: "Original" }],
      );
      const me = await client.getMe();
      await client.updateUserSettings({ settings: { ...me.settings, language: "de" } });
      await page.reload();
      await waitForWorkspace(page);
      await page.locator(`[data-testid="sidebar-node-${tableId}"]`).click();
      if (preference === "zoom") {
        await worker.evaluate("chrome.tabs.setZoom(2)");
        await expect.poll(() => page.evaluate(() => innerWidth)).toBe(640);
        await page.setViewportSize({ width: 780, height: 900 });
        await expect.poll(() => page.evaluate(() => innerWidth)).toBe(390);
      } else {
        await page.setViewportSize({ width: 390, height: 900 });
      }
      const notNow = page.getByRole("button", { name: "Nicht jetzt" });
      if (await notNow.isVisible()) await notNow.click();
      const ws = client.ws(wsID);
      const record = (await ws.listRecords(tableId, { ViewID: "", Filters: "", Sorts: "", Offset: 0, Limit: 100 }))
        .records?.[0];
      const schema = await ws.getNode(tableId);
      if (!record || !schema.schema_revision) throw new Error("Missing record or schema");
      let release!: () => void;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      await page.route(`**/nodes/${tableId}/table/records/${record.id}`, async (route) => {
        if (route.request().method() === "POST") await gate;
        await route.continue();
      });
      await page.locator("td").getByText("Original", { exact: true }).click();
      const input = page.locator('table td input[type="text"]').first();
      await input.fill("My draft");
      const sent = page.waitForRequest((req) => req.method() === "POST" && req.url().endsWith(`/records/${record.id}`));
      await input.press("Enter");
      await sent;
      await ws.updateRecord(tableId, record.id, {
        data: { Name: "Remote" },
        expected_revision: record.revision,
        expected_schema_revision: schema.schema_revision,
      });
      release();
      const draft = page.getByRole("alert").filter({ hasText: "Dieser Datensatz konnte nicht gespeichert werden" });
      await expect(draft).toBeVisible();
      await draft.getByRole("button", { name: "Aktuellen Datensatz prüfen" }).click();
      await expect(draft.locator("pre").filter({ hasText: "Remote" })).toBeVisible();
      const geometry = await draft.evaluate((el) => ({
        right: el.getBoundingClientRect().right,
        scroll: el.scrollWidth,
        width: el.clientWidth,
      }));
      expect(geometry.right).toBeLessThanOrEqual(390);
      expect(geometry.scroll).toBeLessThanOrEqual(geometry.width);
      await expect(draft.getByRole("button", { name: "Durch meine Änderungen ersetzen" })).toBeVisible();
      await expect(draft.getByRole("button", { name: "Entwurf verwerfen" })).toBeVisible();
      for (const button of await draft.getByRole("button").all()) {
        const metrics = await button.evaluate((element) => ({
          font: getComputedStyle(element).fontSize,
          parent: getComputedStyle(element.parentElement!).fontSize,
          right: element.getBoundingClientRect().right,
        }));
        expect(metrics.font).toBe(metrics.parent);
        expect(metrics.right).toBeLessThanOrEqual(390);
        await button.click({ trial: true });
      }
      await page.screenshot({ path: testInfo.outputPath(`record-draft-${preference.replaceAll(" ", "-")}.png`) });
    } finally {
      await context.close();
    }
  });
}
