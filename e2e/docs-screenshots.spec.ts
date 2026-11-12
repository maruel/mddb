// Documentation scenes that scripts/docs_screenshots.py publishes as lossless WebP baselines.

import { randomUUID } from "node:crypto";
import type { APIRequestContext, Page } from "@playwright/test";
import type { VoiceChoicesResp } from "@maruel/gomode/sdk/voicegateway/v2/types.gen.ts";
import { test, expect, createClient, gotoWithSession, registerUser, waitForWorkspace } from "./helpers";
import { nodeUrl } from "../frontend/src/utils/urls";

// Fixed viewport, scheme, locale, timezone, and clock keep every pixel repeatable. Partial raster repaints
// only invalidated tile regions, which makes antialiased rounded edges depend on paint history.
test.use({
  viewport: { width: 1280, height: 800 },
  deviceScaleFactor: 1,
  colorScheme: "light",
  locale: "en-US",
  timezoneId: "UTC",
  launchOptions: { args: ["--disable-partial-raster"] },
});

const NOW = new Date("2026-03-02T09:30:00Z");

// The server makes its first account a global administrator, which adds Server Settings to the settings sidebar.
// Every worker registers a filler account before its first scene, so the first account is never a scene's.
test.beforeAll(async ({ request }) => {
  await registerUser(request, "docs-filler");
});

const ROADMAP_PROPERTIES = [
  { name: "Feature", type: "text" as const },
  {
    name: "Status",
    type: "select" as const,
    options: [
      { id: "planned", name: "Planned" },
      { id: "in_progress", name: "In progress" },
      { id: "shipped", name: "Shipped" },
    ],
  },
  {
    name: "Area",
    type: "multi_select" as const,
    options: [
      { id: "editor", name: "Editor" },
      { id: "sync", name: "Sync" },
      { id: "tables", name: "Tables" },
      { id: "voice", name: "Voice" },
    ],
  },
  { name: "Target", type: "date" as const },
  { name: "Estimate", type: "number" as const },
  { name: "Reviewed", type: "checkbox" as const },
];

const ROADMAP_RECORDS = [
  ["Offline editing", "in_progress", "editor,sync", "2026-04-15", 8, "true"],
  ["Board view swimlanes", "planned", "tables", "2026-05-01", 5, "false"],
  ["Git push on save", "shipped", "sync", "2026-02-10", 3, "true"],
  ["Voice page summaries", "in_progress", "voice", "2026-04-30", 13, "false"],
  ["Formula columns", "planned", "tables", "2026-06-12", 8, "false"],
  ["Markdown import", "shipped", "editor", "2026-01-20", 2, "true"],
  ["Shared templates", "planned", "editor,tables", "2026-06-30", 5, "false"],
] as const;

function launchPlan(roadmapLink: string): string {
  return `## Goals

Ship the spring release to every workspace by **April 30**. Each team owns one milestone and tracks it in the
[Roadmap](${roadmapLink}).

## Milestones

1. Freeze the editor and table features.
2. Run the beta with three partner teams.
3. Publish the release notes and the migration guide.

## Checklist

- [x] Draft the announcement
- [x] Review the accessibility of new dialogs
- [ ] Record the product walkthrough
- [ ] Update the help center

> Every page is a Markdown file in a Git repository, so the plan keeps its full history.

\`\`\`bash
git clone https://github.com/acme/handbook.git
\`\`\`
`;
}

interface DemoWorkspace {
  workspaceId: string;
  launchPlanId: string;
  roadmapId: string;
  tableViewId: string;
  boardViewId: string;
}

// Each scene owns an account with a fixed display name; no scene shows its unique email.
async function openDemoWorkspace(page: Page, request: APIRequestContext): Promise<DemoWorkspace> {
  const auth = await createClient(request).register({
    email: `alex.morgan-${randomUUID()}@example.com`,
    password: "demo-password-1",
    name: "Alex Morgan",
  });
  const client = createClient(request, auth.token);
  await client.updateUserSettings({ settings: { theme: "light", language: "en" } });
  await page.clock.setFixedTime(NOW);
  // Model an already dismissed installation offer; it has its own foreground journey.
  await page.addInitScript(() => localStorage.setItem("mddb_pwa_dismissed", "2026-03-01T00:00:00.000Z"));
  // Remote web fonts would make pixels depend on the network; the installed Noto Color Emoji renders icons.
  await page.route(/^https:\/\/fonts\.(googleapis|gstatic)\.com\//, (route) => route.abort());
  await gotoWithSession(page, auth.token);
  const workspaceId = await waitForWorkspace(page);
  const me = await client.getMe();
  await client.org(me.organization_id!).updateOrganization({ name: "Acme" });
  const ws = client.ws(workspaceId);
  await ws.updateWorkspace({ name: "Product team" });

  // Create nodes serially: the sidebar lists siblings in creation order.
  const createPage = async (parent: string, title: string, icon: string, content: string) => {
    const created = await ws.createPage(parent, { title, content });
    await ws.updatePageFrontmatter(created.id, { icon, cover: "" });
    return created.id;
  };
  const handbook = await createPage("0", "Team handbook", "📘", "How the product team plans, ships, and reviews work.");
  await createPage(handbook, "Onboarding", "👋", "Accounts, tools, and the first week.");
  await createPage(handbook, "Meeting rituals", "🗓️", "Weekly planning, demos, and retrospectives.");
  const roadmap = await ws.createTable("0", { title: "Roadmap", properties: [...ROADMAP_PROPERTIES] });
  for (const [feature, status, area, target, estimate, reviewed] of ROADMAP_RECORDS) {
    await ws.createRecord(roadmap.id, {
      data: { Feature: feature, Status: status, Area: area, Target: target, Estimate: estimate, Reviewed: reviewed },
    });
  }
  // The first saved view replaces the implicit one, so save both.
  const table = await ws.createView(roadmap.id, { name: "All features", type: "table" });
  const board = await ws.createView(roadmap.id, { name: "By status", type: "board" });
  const launchPlanId = await createPage(
    "0",
    "Spring launch plan",
    "🚀",
    launchPlan(nodeUrl(workspaceId, "Product team", roadmap.id, "Roadmap")),
  );
  await createPage("0", "Meeting notes", "📝", "Decisions and follow-ups from each weekly sync.");
  await ws.updateGitRemote({ url: "https://github.com/acme/handbook.git", type: "github", auth_type: "token" });
  return {
    workspaceId,
    launchPlanId,
    roadmapId: roadmap.id,
    tableViewId: table.view.id,
    boardViewId: board.view.id,
  };
}

// Wait for the live connection, then park the pointer and focus where they draw no hover or focus state.
async function settleWorkspace(page: Page) {
  await expect(page.getByRole("banner").getByRole("status")).toHaveText("Connected");
  await settle(page);
}

// Park the pointer and focus where they draw no hover or focus state.
async function settle(page: Page) {
  await page.mouse.move(1279, 799);
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
}

test.screenshot("document page in the editor", async ({ page, request, takeScreenshot }) => {
  const demo = await openDemoWorkspace(page, request);
  await page.goto(nodeUrl(demo.workspaceId, undefined, demo.launchPlanId));
  const editor = page.getByTestId("wysiwyg-editor");
  await expect(editor.getByText("Update the help center")).toBeVisible();
  await expect(editor.getByRole("link", { name: "Roadmap", exact: true })).toBeVisible();
  await settleWorkspace(page);
  await takeScreenshot("document-editor");
});

test.screenshot("roadmap table view", async ({ page, request, takeScreenshot }) => {
  const demo = await openDemoWorkspace(page, request);
  await page.goto(`${nodeUrl(demo.workspaceId, undefined, demo.roadmapId)}?view=${demo.tableViewId}`);
  await expect(page.locator("table").getByText("Shared templates")).toBeVisible();
  await settleWorkspace(page);
  await takeScreenshot("table-view");
});

test.screenshot("roadmap board view", async ({ page, request, takeScreenshot }) => {
  const demo = await openDemoWorkspace(page, request);
  await page.goto(`${nodeUrl(demo.workspaceId, undefined, demo.roadmapId)}?view=${demo.boardViewId}`);
  await expect(page.getByTestId("board-column")).toHaveCount(3);
  await settleWorkspace(page);
  await takeScreenshot("board-view");
});

test.screenshot("workspace Git sync settings", async ({ page, request, takeScreenshot }) => {
  const demo = await openDemoWorkspace(page, request);
  await page.goto(`/settings/workspace/${demo.workspaceId}`);
  const gitTab = page.getByRole("button", { name: "Git Sync", exact: true });
  await gitTab.click();
  await expect(gitTab).toHaveClass(/active/i);
  await settle(page);
  await takeScreenshot("settings-git-sync");
});

test.screenshot("voice settings over a document", async ({ page, request, baseURL, takeScreenshot }) => {
  await page.route("**/.well-known/gomode.json", async (route) => {
    const response = await route.fetch();
    const manifest = await response.json();
    manifest.webShell.voiceGateway = { url: baseURL! };
    await route.fulfill({ response, json: manifest });
  });
  await page.route("**/api/voicegateway/v2/voice/choices", (route) =>
    route.fulfill({
      json: {
        choices: [{ id: "live", label: "Gemini Live", transports: ["webrtc"] }],
        defaults: [{ transport: "webrtc", id: "live" }],
      } satisfies VoiceChoicesResp,
    }),
  );
  const demo = await openDemoWorkspace(page, request);
  await page.goto(nodeUrl(demo.workspaceId, undefined, demo.launchPlanId));
  await expect(page.getByTestId("wysiwyg-editor").getByText("Update the help center")).toBeVisible();
  await expect(page.getByTestId("voice-overlay")).toBeVisible();
  await page.getByRole("button", { name: "Voice settings", exact: true }).click();
  await expect(page.getByRole("combobox", { name: "Model", exact: true })).toHaveValue("live");
  await settleWorkspace(page);
  await takeScreenshot("voice-settings");
});
