// Exercises derived draft dirtiness, serialized acknowledgements, protected departure, and explicit conflict recovery.

import {
  test,
  expect,
  registerUser,
  createClient,
  waitForWorkspace,
  gotoWithSession,
  fillEditorContent,
} from "./helpers";
import { gate, installNativeTools, invokeNativeFocus, nativeResult, waitNativeReady } from "./gomode-fixtures";
import type { UpdatePageRequest } from "../sdk/mddb/ts/v1/types.gen";

test("returning title and native body undo to acknowledged text avoids a draft write", async ({ page, request }) => {
  const { token } = await registerUser(request, "draft-undo-clean");
  await gotoWithSession(page, token);
  const wsID = await waitForWorkspace(page);
  const ws = createClient(request, token).ws(wsID);
  const source = await ws.createPage("0", { title: "Original title", content: "Original body" });
  const target = await ws.createPage("0", { title: "Other page", content: "Other body" });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  const paragraph = page.getByTestId("wysiwyg-editor").locator(".ProseMirror p").first();
  await expect(title).toHaveValue("Original title");
  await expect(paragraph).toHaveText("Original body");
  let writes = 0;
  page.on("request", (req) => {
    if (req.method() === "POST" && req.url().endsWith(`/nodes/${source.id}/page`)) writes++;
  });
  await page.clock.install();
  await page.clock.pauseAt(new Date());
  await title.fill("Edited title");
  await title.fill("Original title");
  await expect(page.getByText("Unsaved", { exact: true })).not.toBeVisible();
  await paragraph.click();
  await page.keyboard.press("Control+End");
  await page.keyboard.type(" added text");
  await expect(page.getByText("Unsaved", { exact: true })).toBeVisible();
  await page.keyboard.press("Control+z");
  await expect(paragraph).toHaveText("Original body");
  await expect(page.getByText("Unsaved", { exact: true })).not.toBeVisible();
  await page.clock.runFor(2100);
  expect(writes).toBe(0);
  await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
  await expect(page).toHaveURL(new RegExp(`/@${target.id}(?:\\+|$)`));
  expect(writes).toBe(0);
  await page.clock.resume();
});

test("a closed comparison keeps review available for a clean draft and writes an explicit old baseline", async ({
  page,
  request,
}) => {
  const { token } = await registerUser(request, "draft-review-clean");
  await gotoWithSession(page, token);
  const wsID = await waitForWorkspace(page);
  const ws = createClient(request, token).ws(wsID);
  const source = await ws.createPage("0", { title: "Original title", content: "Original body" });
  const target = await ws.createPage("0", { title: "Other page", content: "Other body" });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  await expect(title).toHaveValue("Original title");
  await page.clock.install();
  await page.clock.pauseAt(new Date());
  let writes = 0;
  page.on("request", (req) => {
    if (req.method() === "POST" && req.url().endsWith(`/nodes/${source.id}/page`)) writes++;
  });
  await title.fill("Review draft");
  const latest = await ws.updatePage(source.id, {
    title: "Remote title",
    content: "Remote body",
    expected_revision: (await ws.getNode(source.id)).revision ?? "",
  });
  const compare = page.getByRole("button", { name: "Compare changes", exact: true });
  await compare.click();
  const dialog = page.getByRole("dialog", { name: "Compare changes", exact: true });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await title.fill("Original title");
  await expect(page.getByText("Unsaved", { exact: true })).not.toBeVisible();
  await expect(page.getByText("Review changes or reload the latest page to continue.", { exact: true })).toBeVisible();
  await expect(compare).toBeVisible();
  await expect(page.getByRole("button", { name: "Dismiss", exact: true })).not.toBeVisible();
  await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
  await expect(page).toHaveURL(new RegExp(`/@${source.id}(?:\\+|$)`));
  expect(writes).toBe(0);
  await compare.click();
  const result = dialog.getByRole("group", { name: "Recovered result" });
  await expect(result.getByRole("textbox", { name: "Title", exact: true })).toHaveValue("Remote title");
  await result.getByRole("textbox", { name: "Title", exact: true }).fill("Original title");
  await result.getByRole("textbox", { name: "Body", exact: true }).fill("Original body");
  const recovering = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().endsWith(`/nodes/${source.id}/page`),
  );
  await dialog.getByRole("button", { name: "Save recovered page", exact: true }).click();
  expect((await recovering).postDataJSON()).toEqual({
    title: "Original title",
    content: "Original body",
    expected_revision: latest.node.revision,
  });
  await expect(dialog).not.toBeVisible();
  await expect(title).toHaveValue("Original title");
  expect(writes).toBe(1);
  expect((await ws.getNode(source.id)).title).toBe("Original title");
  await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
  await expect(page).toHaveURL(new RegExp(`/@${target.id}(?:\\+|$)`));
  await page.clock.resume();
});

test("a lost save response keeps the draft for inspection without resubmitting", async ({ page, request }) => {
  const { token } = await registerUser(request, "conditional-lost-response");
  await gotoWithSession(page, token);
  const wsID = await waitForWorkspace(page);
  const ws = createClient(request, token).ws(wsID);
  const source = await ws.createPage("0", { title: "Uncertain source", content: "Original body" });
  const target = await ws.createPage("0", { title: "Uncertain target", content: "Other body" });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  await expect(title).toHaveValue("Uncertain source");
  let writes = 0;
  await page.route(`**/nodes/${source.id}/page`, async (route) => {
    writes++;
    await route.fetch();
    await route.abort("failed");
  });
  await title.fill("Uncertain draft");
  await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
  await expect(page.getByText("The save could not be confirmed.", { exact: false }).first()).toBeVisible();
  await expect(title).toHaveValue("Uncertain draft");
  expect((await ws.getNode(source.id)).title).toBe("Uncertain draft");
  await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
  await expect(page).toHaveURL(new RegExp(`/@${source.id}(?:\\+|$)`));
  expect(writes).toBe(1);
  await page.getByRole("button", { name: "Discard edits and reload", exact: true }).click();
  await expect(page.getByRole("button", { name: "Copy draft", exact: true })).not.toBeVisible();
});

test("a failed latest read after an uncertain recovery save keeps unknown-outcome protection", async ({
  page,
  request,
}) => {
  const { token } = await registerUser(request, "recovery-unknown-read");
  await gotoWithSession(page, token);
  const wsID = await waitForWorkspace(page);
  const ws = createClient(request, token).ws(wsID);
  const source = await ws.createPage("0", { title: "Original", content: "Original body" });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  await expect(title).toHaveValue("Original");
  await page.clock.install();
  await page.clock.pauseAt(new Date());
  await title.fill("Private recovery draft");
  await ws.updatePage(source.id, {
    title: "Remote title",
    content: "Original body",
    expected_revision: (await ws.getNode(source.id)).revision ?? "",
  });
  const conflict = page.waitForResponse(
    (response) => response.url().endsWith(`/nodes/${source.id}/page`) && response.status() === 409,
  );
  await page.clock.runFor(2100);
  await conflict;
  await page.getByRole("button", { name: "Compare changes", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Compare changes" });
  await dialog.getByRole("radio", { name: "Use draft" }).check();
  let recoveryWrites = 0;
  await page.route(`**/nodes/${source.id}/page`, async (route) => {
    recoveryWrites++;
    await route.fetch();
    await route.abort("failed");
  });
  await dialog.getByRole("button", { name: "Save recovered page", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("recovered draft is preserved");
  await page.route(`**/nodes/${source.id}`, (route) =>
    route.fulfill({ status: 403, json: { error: { code: "FORBIDDEN", message: "Read forbidden" } } }),
  );
  await dialog.getByRole("button", { name: "Load latest version", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("Could not load the latest version");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page.getByText("The save could not be confirmed.", { exact: false }).first()).toBeVisible();
  await expect(page.getByRole("button", { name: "Compare changes", exact: true })).not.toBeVisible();
  await expect(title).toHaveValue("Private recovery draft");
  await title.fill("Still private after failed inspection");
  await page.clock.runFor(2100);
  expect(recoveryWrites).toBe(1);
  await expect(title).toHaveValue("Still private after failed inspection");
});

test("the old editor cannot accept edits while the destination load is pending", async ({ page, request }) => {
  const { token } = await registerUser(request, "conditional-departure-load");
  await gotoWithSession(page, token);
  const wsID = await waitForWorkspace(page);
  const ws = createClient(request, token).ws(wsID);
  const source = await ws.createPage("0", { title: "Departure source", content: "Original body" });
  const target = await ws.createPage("0", { title: "Departure target", content: "Other body" });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  await expect(title).toHaveValue("Departure source");
  const loaded = gate(),
    reply = gate();
  await page.route(`**/nodes/${target.id}`, async (route) => {
    const response = await route.fetch();
    loaded.resolve();
    await reply.promise;
    await route.fulfill({ response });
  });
  try {
    await title.fill("Saved before departure");
    await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
    await loaded.promise;
    await expect(title).toBeDisabled();
    await expect(page.locator("[inert][aria-busy=true]")).toBeVisible();
    await expect(page.locator(".ProseMirror")).toHaveAttribute("contenteditable", "false");
    expect((await ws.getNode(source.id)).title).toBe("Saved before departure");
    reply.resolve();
    await expect(title).toHaveValue("Departure target");
    await expect(title).toBeEnabled();
    await expect(page.locator(".ProseMirror")).toHaveAttribute("contenteditable", "true");
  } finally {
    reply.resolve();
  }
});

test("navigation waits for both serialized saves and uses the acknowledged revision", async ({ page, request }) => {
  const { token } = await registerUser(request, "conditional-queue");
  await gotoWithSession(page, token);
  const wsID = await waitForWorkspace(page);
  const ws = createClient(request, token).ws(wsID);
  const source = await ws.createPage("0", { title: "Queue source", content: "body" });
  const target = await ws.createPage("0", { title: "Queue target", content: "target" });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  await expect(title).toHaveValue("Queue source");
  const firstWritten = gate(),
    firstReply = gate(),
    secondWritten = gate(),
    secondReply = gate();
  const requests: UpdatePageRequest[] = [];
  let firstRevision = "";
  await page.route(`**/nodes/${source.id}/page`, async (route) => {
    requests.push(route.request().postDataJSON() as UpdatePageRequest);
    const response = await route.fetch();
    if (requests.length === 1) {
      firstRevision = ((await response.json()) as { node: { revision: string } }).node.revision;
      firstWritten.resolve();
      await firstReply.promise;
    } else {
      secondWritten.resolve();
      await secondReply.promise;
    }
    await route.fulfill({ response });
  });
  try {
    await title.fill("First draft");
    await firstWritten.promise;
    await title.fill("Second draft");
    await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
    expect(requests).toHaveLength(1);
    await expect(page).toHaveURL(new RegExp(`/@${source.id}(?:\\+|$)`));
    firstReply.resolve();
    await secondWritten.promise;
    expect(requests[1]?.expected_revision).toBe(firstRevision);
    expect(requests[1]?.title).toBe("Second draft");
    await expect(page).toHaveURL(new RegExp(`/@${source.id}(?:\\+|$)`));
    secondReply.resolve();
    await expect(page).toHaveURL(new RegExp(`/@${target.id}(?:\\+|$)`));
    expect((await ws.getNode(source.id)).title).toBe("Second draft");
  } finally {
    firstReply.resolve();
    secondReply.resolve();
  }
});

test("a conflict preserves draft and focus until explicit copy and discard recovery", async ({
  page,
  context,
  request,
}) => {
  const { token } = await registerUser(request, "conditional-conflict");
  await installNativeTools(page, request);
  await gotoWithSession(page, token);
  await waitNativeReady(page);
  const wsID = await waitForWorkspace(page);
  const ws = createClient(request, token).ws(wsID);
  const source = await ws.createPage("0", { title: "Conflict source", content: "Original body" });
  const target = await ws.createPage("0", { title: "Conflict target", content: "Other body" });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  await waitNativeReady(page);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  await expect(title).toHaveValue("Conflict source");
  await page.clock.install();
  await page.clock.pauseAt(new Date());
  await title.fill("Private draft");
  const latest = await ws.updatePage(source.id, {
    title: "External title",
    content: "External body",
    expected_revision: (await ws.getNode(source.id)).revision ?? "",
  });
  const conflicted = page.waitForResponse(
    (response) => response.url().endsWith(`/nodes/${source.id}/page`) && response.status() === 409,
  );
  await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
  await conflicted;
  await expect(page.getByRole("button", { name: "Copy draft", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Dismiss", exact: true })).not.toBeVisible();
  await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
  await expect(page.getByRole("button", { name: "Discard edits and reload", exact: true })).toBeVisible();
  await expect(title).toHaveValue("Private draft");
  await expect(page).toHaveURL(new RegExp(`/@${source.id}(?:\\+|$)`));
  await invokeNativeFocus(page, target.id, "conflicted-focus", "conditional-conversation");
  expect(await nativeResult(page, "conflicted-focus")).toHaveProperty("error");
  await expect(page).toHaveURL(new RegExp(`/@${source.id}(?:\\+|$)`));
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.getByRole("button", { name: "Copy draft", exact: true }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe("# Private draft\n\nOriginal body");
  expect((await ws.getNode(source.id)).revision).toBe(latest.node.revision);
  await page.getByRole("button", { name: "Discard edits and reload", exact: true }).click();
  await expect(title).toHaveValue("External title");
  await expect(page.getByRole("button", { name: "Copy draft", exact: true })).not.toBeVisible();
  const saving = page.waitForResponse(
    (response) => response.url().endsWith(`/nodes/${source.id}/page`) && response.ok(),
  );
  await title.fill("Recovered edit");
  await page.clock.runFor(2100);
  await saving;
  expect((await ws.getNode(source.id)).title).toBe("Recovered edit");
});

for (const scenario of ["independent", "overlapping"] as const) {
  test(`compares ${scenario} edits and conditionally saves a reviewed result`, async ({ page, request }, testInfo) => {
    await page.setViewportSize(scenario === "independent" ? { width: 1280, height: 900 } : { width: 390, height: 844 });
    const { token } = await registerUser(request, `recover-${scenario}`);
    await gotoWithSession(page, token);
    const wsID = await waitForWorkspace(page);
    const ws = createClient(request, token).ws(wsID);
    const source = await ws.createPage("0", { title: "Original title", content: "one\ntwo\nthree\n" });
    await page.goto(`/w/@${wsID}/@${source.id}`);
    const title = page.getByRole("textbox", { name: "Title", exact: true });
    await expect(title).toHaveValue("Original title");
    await page.clock.install();
    await page.clock.pauseAt(new Date());
    await fillEditorContent(page, scenario === "independent" ? "ONE\ntwo\nthree\n" : "one\nlocal passage\nthree\n");
    if (scenario === "overlapping") await title.fill("Draft title");
    const latest = await ws.updatePage(source.id, {
      title: "Saved title",
      content: scenario === "independent" ? "one\ntwo\nTHREE\n" : "one\nsaved passage\nthree\n",
      expected_revision: (await ws.getNode(source.id)).revision ?? "",
    });
    const conflicted = page.waitForResponse(
      (response) => response.url().endsWith(`/nodes/${source.id}/page`) && response.status() === 409,
    );
    await page.clock.runFor(2100);
    await conflicted;
    const compareButton = page.getByRole("button", { name: "Compare changes", exact: true });
    await compareButton.click();
    let dialog = page.getByRole("dialog", { name: "Compare changes" });
    await expect(dialog.getByRole("heading", { name: "Your draft" })).toBeVisible();
    const result = dialog.getByRole("group", { name: "Recovered result" });
    if (scenario === "overlapping") {
      await expect(dialog.getByRole("button", { name: "Save recovered page" })).toBeDisabled();
      await dialog.getByRole("radio", { name: "Use draft" }).first().check();
      await dialog.getByRole("radio", { name: "Use saved version" }).last().check();
      await expect(result.getByRole("textbox", { name: "Title", exact: true })).toHaveValue("Draft title");
      await expect(result.getByRole("textbox", { name: "Body", exact: true })).toHaveValue(
        "one\nsaved passage\nthree\n",
      );
    } else {
      await expect(dialog.getByRole("radio")).toHaveCount(0);
      await expect(result.getByRole("textbox", { name: "Body", exact: true })).toHaveValue("ONE\ntwo\nTHREE\n");
    }
    // A keyboard user can cancel, return to the trigger, and reopen without losing edits.
    await page.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible();
    await expect(compareButton).toBeFocused();
    await page.keyboard.press("Enter");
    dialog = page.getByRole("dialog", { name: "Compare changes" });
    await expect(dialog.getByRole("heading", { name: "Your draft" })).toBeVisible();
    if (scenario === "overlapping") {
      await dialog.getByRole("radio", { name: "Use draft" }).first().check();
      await dialog.getByRole("radio", { name: "Use saved version" }).last().check();
    }
    await expect(dialog).toHaveJSProperty("scrollWidth", await dialog.evaluate((element) => element.clientWidth));
    await page.screenshot({
      path: testInfo.outputPath(`recovery-${scenario}.png`),
      fullPage: true,
      animations: "disabled",
    });
    const body = dialog.getByRole("textbox", { name: "Body", exact: true });
    await body.fill(`${await body.inputValue()}Reviewed addition\n`);
    let expectedRevision = latest.node.revision;
    if (scenario === "overlapping") {
      const newer = await ws.updatePage(source.id, {
        title: "New writer",
        content: "Newest saved body",
        expected_revision: latest.node.revision ?? "",
      });
      const again = page.waitForResponse(
        (response) => response.url().endsWith(`/nodes/${source.id}/page`) && response.status() === 409,
      );
      await dialog.getByRole("button", { name: "Save recovered page" }).click();
      await again;
      await expect(dialog.getByRole("alert")).toContainText("recovered draft is preserved");
      await dialog.getByRole("button", { name: "Load latest version" }).click();
      await expect(dialog.getByRole("heading", { name: "Latest saved version" }).locator("..")).toContainText(
        "Newest saved body",
      );
      await dialog.getByRole("radio", { name: "Use draft" }).first().check();
      await dialog.getByRole("radio", { name: "Use draft" }).last().check();
      expectedRevision = newer.node.revision;
    }
    const saving = page.waitForRequest(
      (req) => req.url().endsWith(`/nodes/${source.id}/page`) && req.method() === "POST",
    );
    await dialog.getByRole("button", { name: "Save recovered page" }).click();
    expect(((await saving).postDataJSON() as UpdatePageRequest).expected_revision).toBe(expectedRevision);
    await expect(dialog).not.toBeVisible();
    await expect(title).toBeFocused();
    expect((await ws.getNode(source.id)).content).toContain("Reviewed addition");
  });
}

for (const locale of ["en", "de"] as const) {
  test(`changed regions and keyboard overlap navigation preserve a long ${locale} recovery`, async ({
    page,
    request,
  }, testInfo) => {
    await page.setViewportSize(locale === "en" ? { width: 1280, height: 900 } : { width: 390, height: 844 });
    const labels =
      locale === "en"
        ? {
            compare: "Compare changes",
            draft: "Your draft",
            saved: "Latest saved version",
            base: "Last acknowledged version",
            previous: "Previous unresolved overlap",
            next: "Next unresolved overlap",
            count: "unresolved overlaps",
            useDraft: "Use draft",
            useSaved: "Use saved version",
            body: "Body",
            save: "Save recovered page",
            previousResult: "Previous recovery result — select and copy any edits you want to keep",
          }
        : {
            compare: "Änderungen vergleichen",
            draft: "Ihr Entwurf",
            saved: "Zuletzt gespeicherte Version",
            base: "Zuletzt bestätigte Version",
            previous: "Vorherige ungelöste Überschneidung",
            next: "Nächste ungelöste Überschneidung",
            count: "ungelöste Überschneidungen",
            useDraft: "Entwurf verwenden",
            useSaved: "Gespeicherte Version verwenden",
            body: "Inhalt",
            save: "Wiederhergestellte Seite speichern",
            previousResult: "Vorheriges Wiederherstellungsergebnis — gewünschte Änderungen auswählen und kopieren",
          };
    const { token } = await registerUser(request, `recovery-regions-${locale}`);
    const client = createClient(request, token);
    if (locale === "de") await client.updateUserSettings({ settings: { theme: "system", language: locale } });
    await gotoWithSession(page, token);
    const wsID = await waitForWorkspace(page);
    const ws = client.ws(wsID);
    const original = Array.from({ length: 180 }, (_, i) => `Unchanged section ${i} 🌳\n`);
    const ours = [...original],
      theirs = [...original];
    ours[10] = "Draft first passage\n";
    theirs[10] = "Saved first passage\n";
    ours[120] = "Draft last passage\n";
    theirs[120] = "Saved last passage\n";
    ours[150] = "";
    theirs[160] = `Saved independent ${"long ".repeat(80)}\n`;
    const source = await ws.createPage("0", { title: "Region review", content: original.join("") });
    await page.goto(`/w/@${wsID}/@${source.id}`);
    await expect(page.getByTestId("wysiwyg-editor")).toContainText("Unchanged section 0");
    await page.clock.install();
    await page.clock.pauseAt(new Date());
    await fillEditorContent(page, ours.join(""));
    await ws.updatePage(source.id, {
      title: "Region review",
      content: theirs.join(""),
      expected_revision: (await ws.getNode(source.id)).revision ?? "",
    });
    const conflict = page.waitForResponse(
      (response) => response.url().endsWith(`/nodes/${source.id}/page`) && response.status() === 409,
    );
    await page.clock.runFor(2100);
    await conflict;
    await page.getByRole("button", { name: labels.compare, exact: true }).click();
    const dialog = page.getByRole("dialog", { name: labels.compare });
    const local = dialog.getByTestId("recovery-local-source"),
      saved = dialog.getByTestId("recovery-saved-source");
    await expect.poll(() => local.textContent()).toBe(ours.join(""));
    await expect.poll(() => saved.textContent()).toBe(theirs.join(""));
    await expect(local.locator("mark")).toHaveText(["Draft first passage\n", "Draft last passage\n"]);
    await expect(saved.locator("mark")).toHaveText(["Saved first passage\n", "Saved last passage\n", theirs[160]!]);
    await expect(dialog.getByRole("status")).toContainText(`2 ${labels.count}`);
    await page.screenshot({
      path: testInfo.outputPath(`recovery-regions-${locale}-navigation.png`),
      animations: "disabled",
    });
    const next = dialog.getByRole("button", { name: labels.next, exact: true });
    const previous = dialog.getByRole("button", { name: labels.previous, exact: true });
    // Reach the native navigation buttons through the dialog's ordinary Tab order.
    for (let i = 0; i < 6 && !(await next.evaluate((el) => el === document.activeElement)); i++)
      await page.keyboard.press("Tab");
    await expect(next).toBeFocused();
    await next.press("Enter");
    const firstDraft = dialog.getByRole("radio", { name: labels.useDraft }).first();
    const lastDraft = dialog.getByRole("radio", { name: labels.useDraft }).last();
    await expect(firstDraft).toBeFocused();
    await next.press("Enter");
    await expect(lastDraft).toBeFocused();
    await previous.press("Enter");
    await expect(firstDraft).toBeFocused();
    await page.keyboard.press("Space");
    await expect(dialog.getByRole("status")).toContainText(`1 ${labels.count}`);
    await previous.press("Enter");
    await expect(lastDraft).toBeFocused();
    await next.press("Enter");
    await expect(lastDraft).toBeFocused();
    await page.screenshot({
      path: testInfo.outputPath(`recovery-regions-${locale}-overlap.png`),
      animations: "disabled",
    });
    await page.keyboard.press("ArrowRight");
    await expect(dialog.getByRole("radio", { name: labels.useSaved }).last()).toBeChecked();
    await expect(dialog.getByRole("status")).toContainText(`0 ${labels.count}`);
    await expect(next).toBeDisabled();
    const body = dialog.getByRole("textbox", { name: labels.body, exact: true });
    const reviewed = `${await body.inputValue()}Reviewed retained addition\n`;
    await body.fill(reviewed);
    await dialog.getByText(labels.base, { exact: true }).click();
    await expect.poll(() => dialog.getByTestId("recovery-base-source").textContent()).toBe(original.join(""));
    await expect(dialog.getByTestId("recovery-base-source").locator("mark")).toContainText([original[150]!]);
    await expect(body).toHaveValue(reviewed);
    await firstDraft.check();
    await expect(body).toHaveValue(reviewed);
    await dialog.getByRole("radio", { name: labels.useSaved }).first().check();
    await expect(dialog.getByRole("textbox", { name: labels.previousResult, exact: true })).toHaveValue(
      `# Region review\n\n${reviewed}`,
    );
    await body.fill(`${await body.inputValue()}Reviewed retained addition\n`);
    await dialog.getByRole("heading", { name: labels.compare }).scrollIntoViewIfNeeded();
    await expect(dialog).toHaveJSProperty("scrollWidth", await dialog.evaluate((el) => el.clientWidth));
    await page.screenshot({
      path: testInfo.outputPath(`recovery-regions-${locale}-sources.png`),
      animations: "disabled",
    });
    await dialog.getByRole("button", { name: labels.save, exact: true }).click();
    await expect(dialog).not.toBeVisible();
    expect((await ws.getNode(source.id)).content).toContain("Reviewed retained addition");
    expect((await ws.getNode(source.id)).content).not.toContain("Unchanged section 150");
  });
}

for (const preference of ["native zoom", "default font"] as const) {
  test(`recovery remains usable with enlarged ${preference}`, async ({ browser, request, baseURL }, testInfo) => {
    const { mkdir, writeFile } = await import("node:fs/promises");
    const path = await import("node:path");
    const extension = testInfo.outputPath("preferences-extension");
    await mkdir(extension, { recursive: true });
    await writeFile(
      path.join(extension, "manifest.json"),
      JSON.stringify({
        manifest_version: 3,
        name: "Recovery accessibility preferences",
        version: "1.0",
        permissions: ["fontSettings"],
        background: { service_worker: "background.js" },
      }),
    );
    await writeFile(path.join(extension, "background.js"), "chrome.runtime.onInstalled.addListener(() => {});");
    const context = await browser.browserType().launchPersistentContext(testInfo.outputPath("preferences-profile"), {
      channel: "chromium",
      headless: true,
      baseURL,
      viewport: { width: 1280, height: 1000 },
      args: [`--disable-extensions-except=${extension}`, `--load-extension=${extension}`],
    });
    try {
      const worker = context.serviceWorkers()[0] ?? (await context.waitForEvent("serviceworker"));
      const page = context.pages()[0];
      if (!page) throw new Error("Missing recovery browser page");
      await page.goto("/privacy");
      if (preference === "native zoom") {
        const initialDPR = await page.evaluate(() => devicePixelRatio);
        for (const zoom of [1.25, 1.5, 2]) {
          await worker.evaluate(`chrome.tabs.setZoom(${zoom})`);
          await expect.poll(() => page.evaluate(() => innerWidth)).toBe(Math.round(1280 / zoom));
          await expect.poll(() => page.evaluate(() => devicePixelRatio)).toBeCloseTo(initialDPR * zoom);
        }
      } else {
        await worker.evaluate("chrome.fontSettings.setDefaultFontSize({ pixelSize: 24 })");
        await page.reload();
        await expect.poll(() => page.evaluate(() => getComputedStyle(document.documentElement).fontSize)).toBe("24px");
        expect(await page.evaluate(() => innerWidth)).toBe(1280);
      }
      const { token } = await registerUser(request, `recovery-${preference.replaceAll(" ", "-")}`);
      await gotoWithSession(page, token);
      const wsID = await waitForWorkspace(page);
      const ws = createClient(request, token).ws(wsID);
      const source = await ws.createPage("0", { title: "Original title", content: "Original body" });
      await page.goto(`/w/@${wsID}/@${source.id}`);
      const title = page.getByRole("textbox", { name: "Title", exact: true });
      await expect(title).toHaveValue("Original title");
      await page.clock.install();
      await page.clock.pauseAt(new Date());
      await title.fill("Draft title");
      await ws.updatePage(source.id, {
        title: "Saved title",
        content: "Saved body",
        expected_revision: (await ws.getNode(source.id)).revision ?? "",
      });
      const conflict = page.waitForResponse(
        (response) => response.url().endsWith(`/nodes/${source.id}/page`) && response.status() === 409,
      );
      await page.clock.runFor(2100);
      await conflict;
      await page.getByRole("button", { name: "Compare changes", exact: true }).click();
      const dialog = page.getByRole("dialog", { name: "Compare changes" });
      await expect(dialog.getByTestId("recovery-local-source").locator("mark")).toHaveCount(0);
      await expect(dialog.getByTestId("recovery-saved-source").locator("mark")).toHaveText(["Saved body"]);
      await dialog.getByRole("button", { name: "Next unresolved overlap", exact: true }).click();
      await expect(dialog.getByRole("radio", { name: "Use draft" })).toBeFocused();
      await page.keyboard.press("Space");
      await expect(dialog.getByRole("status")).toHaveText("0 unresolved overlaps");
      await expect(dialog).toHaveJSProperty("scrollWidth", await dialog.evaluate((el) => el.clientWidth));
      await dialog.getByRole("heading", { name: "Compare changes" }).scrollIntoViewIfNeeded();
      await page.screenshot({ path: testInfo.outputPath("recovery-preferences-top.png"), animations: "disabled" });
      const body = dialog.getByRole("textbox", { name: "Body", exact: true });
      await body.fill("Reviewed body");
      const save = dialog.getByRole("button", { name: "Save recovered page" });
      await save.scrollIntoViewIfNeeded();
      await page.screenshot({ path: testInfo.outputPath("recovery-preferences-result.png"), animations: "disabled" });
      await save.click();
      await expect(dialog).not.toBeVisible();
      await expect(title).toBeFocused();
      expect((await ws.getNode(source.id)).content).toBe("Reviewed body");
    } finally {
      await context.close();
    }
  });
}
for (const asset of ["body", "cover"] as const) {
  for (const departure of ["node", "failed deletion"] as const) {
    test(`${asset} upload is applied before ${departure}`, async ({ page, request }) => {
      const { token } = await registerUser(request, `upload-${asset}-${departure === "node" ? "node" : "delete"}`);
      await gotoWithSession(page, token);
      const wsID = await waitForWorkspace(page);
      const ws = createClient(request, token).ws(wsID);
      const source = await ws.createPage("0", { title: "Uploading source", content: "Original body" });
      const target = await ws.createPage("0", { title: "Upload destination", content: "Other body" });
      await page.goto(`/w/@${wsID}/@${source.id}`);
      const title = page.getByRole("textbox", { name: "Title", exact: true });
      await expect(title).toHaveValue("Uploading source");
      let pageWrites = 0;
      page.on("request", (call) => {
        if (call.method() === "POST" && call.url().endsWith(`/nodes/${source.id}/page`)) pageWrites++;
      });
      const uploaded = gate(),
        uploadReply = gate(),
        departureStarted = gate(),
        departureReply = gate();
      await page.route(`**/nodes/${source.id}/assets`, async (route) => {
        const response = await route.fetch();
        uploaded.resolve();
        await uploadReply.promise;
        await route.fulfill({ response });
      });
      await page.route(
        departure === "node" ? `**/nodes/${target.id}` : `**/nodes/${source.id}/delete`,
        async (route) => {
          departureStarted.resolve();
          await departureReply.promise;
          if (departure === "node") await route.continue();
          else
            await route.fulfill({
              status: 400,
              json: { error: { code: "VALIDATION_FAILED", message: "Deletion rejected" } },
            });
        },
      );
      try {
        const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"><rect width="1" height="1"/></svg>';
        if (asset === "cover") {
          await page
            .locator('input[type="file"][accept="image/*"]')
            .setInputFiles({ name: "pending.svg", mimeType: "image/svg+xml", buffer: Buffer.from(svg) });
        } else {
          const transfer = await page.evaluateHandle((data) => {
            const dataTransfer = new DataTransfer();
            dataTransfer.items.add(new File([data], "pending.svg", { type: "image/svg+xml" }));
            return dataTransfer;
          }, svg);
          await page.locator(".ProseMirror").dispatchEvent("drop", { dataTransfer: transfer });
          await transfer.dispose();
        }
        await uploaded.promise;
        if (departure === "node") {
          await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
        } else {
          page.once("dialog", (dialog) => dialog.accept());
          const sourceRow = page.getByTestId(`sidebar-node-${source.id}`);
          await sourceRow.hover();
          await sourceRow.getByTestId("delete-node-button").click();
        }
        // Departure must wait for the insertion, leaving its originating editor writable.
        await expect(title).toBeEnabled();
        uploadReply.resolve();
        await departureStarted.promise;
        const saved = await ws.getNode(source.id);
        if (asset === "body") expect(saved.content).toContain("pending.svg");
        else {
          expect(saved.cover).toBe("pending.svg");
          expect(pageWrites).toBe(0);
        }
        departureReply.resolve();
        await expect(title).toHaveValue(departure === "node" ? "Upload destination" : "Uploading source");
        if (departure === "failed deletion") {
          await expect(title).toBeEnabled();
          if (asset === "body") await expect(page.locator('.ProseMirror img[alt="pending.svg"]')).toBeVisible();
        }
      } finally {
        uploadReply.resolve();
        departureReply.resolve();
      }
    });
  }
}

test("an upload failure keeps the source draft and blocks its waiting departure", async ({ page, request }) => {
  const { token } = await registerUser(request, "upload-departure-failure");
  await gotoWithSession(page, token);
  const wsID = await waitForWorkspace(page);
  const ws = createClient(request, token).ws(wsID);
  const source = await ws.createPage("0", { title: "Upload failure source", content: "Original body" });
  const target = await ws.createPage("0", { title: "Upload failure target", content: "Other body" });
  await page.goto(`/w/@${wsID}/@${source.id}`);
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  await expect(title).toHaveValue("Upload failure source");
  const started = gate(),
    reply = gate();
  await page.route(`**/nodes/${source.id}/assets`, async (route) => {
    started.resolve();
    await reply.promise;
    await route.fulfill({
      status: 400,
      json: { error: { code: "VALIDATION_FAILED", message: "Choose a smaller image and upload again" } },
    });
  });
  try {
    await title.fill("Draft kept after failed upload");
    await page.locator('input[type="file"][accept="image/*"]').setInputFiles({
      name: "failed.svg",
      mimeType: "image/svg+xml",
      buffer: Buffer.from('<svg xmlns="http://www.w3.org/2000/svg"/>'),
    });
    await started.promise;
    await page.getByTestId(`sidebar-node-${target.id}`).locator("> div").first().click();
    reply.resolve();
    await expect(page.getByText("Choose a smaller image and upload again", { exact: false })).toBeVisible();
    await expect(title).toHaveValue("Draft kept after failed upload");
    await expect(page).toHaveURL(new RegExp(`/@${source.id}(?:\\+|$)`));
  } finally {
    reply.resolve();
  }
});
