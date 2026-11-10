// Exercises visible first-workspace retry, canonical readiness, and idempotent provisioning on mobile and desktop.
import { test, expect, registerUser, createClient, gotoWithSession, waitForWorkspace } from "./helpers";
import { gate } from "./gomode-fixtures";

test.use({ serviceWorkers: "block" });

for (const width of [390, 1280]) {
  test(`first-workspace retry waits for canonical roots at ${width}px`, async ({ page, request }, info) => {
    await page.setViewportSize({ width, height: 900 });
    const { token } = await registerUser(request, "startup-retry");
    let attempts = 0;
    await page.route("**/api/v1/organizations/*/workspaces", async (route) => {
      if (route.request().method() !== "POST") return route.continue();
      if (++attempts === 1) {
        await route.fulfill({ status: 404, json: { error: { code: "NOT_FOUND", message: "Preparation failed" } } });
      } else await route.continue();
    });
    const requested = gate();
    const release = gate();
    let held = false;
    await page.route("**/api/v1/workspaces/*/nodes/0/children", async (route) => {
      if (held) return route.continue();
      held = true;
      const response = await route.fetch();
      requested.resolve();
      await release.promise;
      await route.fulfill({ response });
    });
    await gotoWithSession(page, token);
    await expect(page.getByRole("alert")).toContainText("Preparation failed");
    await expect(page.getByRole("button", { name: "Try again", exact: true })).toBeEnabled();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: info.outputPath("startup-error.png"), fullPage: true });
    try {
      await page.getByRole("button", { name: "Try again", exact: true }).click();
      await requested.promise;
      await expect(page.getByTestId("workspace-tree")).toHaveAttribute("aria-busy", "true");
      await expect(page.getByTestId("wysiwyg-editor")).toHaveCount(0);
      await expect(page.locator('[data-testid^="sidebar-node-"]')).toHaveCount(0);
    } finally {
      release.resolve();
    }
    await expect(page.getByTestId("wysiwyg-editor")).toContainText("Welcome to mddb");
    expect(attempts).toBe(2);
    const client = createClient(request, token);
    const user = await client.getMe();
    expect(user.organizations).toHaveLength(1);
    expect(user.workspaces).toHaveLength(1);
    expect((await client.ws(await waitForWorkspace(page)).listNodeChildren("0")).nodes).toHaveLength(1);
  });
}

test("first-workspace provisioning waits for the account's translated input", async ({ page, request }) => {
  const { token } = await registerUser(request, "startup-locale");
  const client = createClient(request, token);
  await client.updateUserSettings({ settings: { theme: "system", language: "fr" } });
  const requested = gate();
  const release = gate();
  let organizations = 0;
  page.on("request", (outgoing) => {
    if (outgoing.method() === "POST" && outgoing.url().endsWith("/api/v1/organizations")) organizations++;
  });
  await page.route(/\/assets\/fr-[^/]+\.js$/, async (route) => {
    requested.resolve();
    await release.promise;
    await route.continue();
  });
  try {
    await gotoWithSession(page, token);
    await requested.promise;
    expect(organizations).toBe(0);
    await expect(page.getByTestId("wysiwyg-editor")).toHaveCount(0);
  } finally {
    release.resolve();
  }
  await expect(page.getByTestId("wysiwyg-editor")).toContainText("Bienvenue sur mddb", { timeout: 15000 });
  const user = await client.getMe();
  expect(user.organizations?.[0]?.organization_name).toBe("Organisation de startup-locale");
  expect(user.workspace_name).toBe("Espace de startup-locale");
  expect(organizations).toBe(1);
});

for (const failure of ["welcome", "final-read"] as const) {
  test(`workspace preparation retains user retry after ${failure} failure`, async ({ page, request }, info) => {
    await page.setViewportSize({ width: failure === "welcome" ? 390 : 1280, height: 900 });
    await page.clock.install();
    const { token } = await registerUser(request, `startup-${failure}`);
    let welcomeAttempts = 0;
    let rootReads = 0;
    await page.route("**/api/v1/workspaces/*/nodes/0/page/create", async (route) => {
      welcomeAttempts++;
      if (failure === "welcome" && welcomeAttempts === 1) {
        await route.fulfill({ status: 404, json: { error: { code: "NOT_FOUND", message: "Welcome unavailable" } } });
      } else await route.continue();
    });
    await page.route("**/api/v1/workspaces/*/nodes/0/children", async (route) => {
      rootReads++;
      if (failure === "final-read" && rootReads === 2) {
        await route.fulfill({ status: 404, json: { error: { code: "NOT_FOUND", message: "Roots unavailable" } } });
      } else await route.continue();
    });
    await gotoWithSession(page, token);
    await expect(page).toHaveURL(/\/w\//, { timeout: 15000 });
    const message = failure === "welcome" ? "Welcome unavailable" : "Roots unavailable";
    await expect(page.getByRole("alert")).toContainText(message);
    await page.clock.fastForward(9000);
    await expect(page.getByRole("alert")).toContainText(message);
    const retry = page.getByRole("button", { name: "Try again", exact: true });
    await expect(retry).toBeEnabled();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: info.outputPath("workspace-preparation-error.png"), fullPage: true });
    await retry.click();
    await expect(page.getByTestId("wysiwyg-editor")).toContainText("Welcome to mddb");
    await expect(page.getByRole("alert")).toHaveCount(0);
    expect(welcomeAttempts).toBe(failure === "welcome" ? 2 : 1);
    expect(rootReads).toBe(failure === "welcome" ? 2 : 3);
    const roots = await createClient(request, token)
      .ws(await waitForWorkspace(page))
      .listNodeChildren("0");
    expect(roots.nodes).toHaveLength(1);
  });
}

test("a successful deep-linked node cannot erase failed preparation or its user retry", async ({ page, request }) => {
  const { token } = await registerUser(request, "startup-deep-link");
  const client = createClient(request, token);
  const org = await client.createOrganization({ name: "Deep link preparation" });
  const workspace = await client.org(org.id).createWorkspace({ name: "Deep link workspace" });
  await client.switchWorkspace({ ws_id: workspace.id });
  const node = await client
    .ws(workspace.id)
    .createPage("0", { title: "Deep-linked page", content: "Existing content" });
  const requested = gate();
  const release = gate();
  await page.route(`**/api/v1/workspaces/${workspace.id}/nodes/${node.id}`, async (route) => {
    const response = await route.fetch();
    requested.resolve();
    await release.promise;
    await route.fulfill({ response });
  });
  let rootReads = 0;
  await page.route("**/api/v1/workspaces/*/nodes/0/children", async (route) => {
    if (++rootReads === 1) {
      await route.fulfill({
        status: 404,
        json: { error: { code: "NOT_FOUND", message: "Initial roots unavailable" } },
      });
    } else await route.continue();
  });
  try {
    await gotoWithSession(page, token, `/w/@${workspace.id}/@${node.id}`);
    await requested.promise;
    await expect(page.getByRole("alert")).toContainText("Initial roots unavailable");
  } finally {
    release.resolve();
  }
  await expect(page.getByTestId("wysiwyg-editor")).toContainText("Existing content");
  await expect(page.getByRole("alert")).toContainText("Initial roots unavailable");
  await page.getByRole("button", { name: "Try again", exact: true }).click();
  await expect(page.getByRole("alert")).toHaveCount(0);
  await expect(page.getByTestId(`sidebar-node-${node.id}`)).toBeVisible();
  expect(rootReads).toBe(2);
  expect((await client.ws(workspace.id).listNodeChildren("0")).nodes).toHaveLength(1);
});
