// Verifies Go Mode discovery, resource reads, and browser MCP writes updating the open page.

import { test, expect, registerUser, getWorkspaceId, createClient } from "./helpers";

const protocolVersion = "2026-07-28";

test("Go Mode discovers mddb, reads its workspace, and updates the page open in the browser", async ({
  page,
  request,
}) => {
  const discovery = await request.get("/.well-known/gomode.json");
  expect(discovery.ok()).toBe(true);
  const settings = await discovery.json();
  expect(settings.service).toBe("mddb");
  expect(settings.webShell.toolGroups).toEqual([
    expect.objectContaining({ endpoint: "/api/v1/gomode/mcp", authRequired: true, protocolVersion }),
  ]);

  const unauthenticated = await request.post("/api/v1/gomode/mcp", {
    data: { jsonrpc: "2.0", id: "1", method: "server/discover", params: {} },
  });
  expect(unauthenticated.status()).toBe(401);

  const { token } = await registerUser(request, "gomode-mcp");
  const client = createClient(request, token);
  await page.goto(`/?token=${token}`);
  await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });
  const wsID = await getWorkspaceId(page);
  const node = await client.ws(wsID).createPage("0", { title: "Go Mode document", content: "Readable through MCP" });
  const child = await client
    .ws(wsID)
    .createPage(node.id, { title: "Nested Go Mode document", content: "Nested content" });

  const call = async (method: string, params: Record<string, unknown>, name?: string) => {
    const { status, body } = await page.evaluate(
      async (input) => {
        const response = await fetch("/api/v1/gomode/mcp", {
          method: "POST",
          headers: input.headers,
          body: JSON.stringify(input.data),
        });
        return { status: response.status, body: await response.json() };
      },
      {
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
          "X-Request-ID": crypto.randomUUID(),
          "Mcp-Protocol-Version": protocolVersion,
          "Mcp-Method": method,
          ...(name ? { "Mcp-Name": name } : {}),
        },
        data: {
          jsonrpc: "2.0",
          id: "1",
          method,
          params: {
            ...params,
            _meta: {
              "io.modelcontextprotocol/protocolVersion": protocolVersion,
              "io.modelcontextprotocol/clientInfo": { name: "mddb-e2e", version: "1" },
              "io.modelcontextprotocol/clientCapabilities": {},
            },
          },
        },
      },
    );
    expect(status).toBe(200);
    expect(body).toEqual(expect.objectContaining({ jsonrpc: "2.0", id: "1", result: expect.any(Object) }));
    return body.result;
  };

  const tools = await call("tools/list", {});
  expect(tools.tools).toEqual(expect.arrayContaining([expect.objectContaining({ name: "node_read" })]));
  const result = await call("tools/call", { name: "node_read", arguments: { nodeId: node.id } }, "node_read");
  expect(result.isError).not.toBe(true);
  expect(result.structuredContent.node).toEqual(
    expect.objectContaining({ title: "Go Mode document", content: "Readable through MCP" }),
  );

  const uri = `mddb://workspaces/${wsID}/nodes/${node.id}`;
  const childURI = `mddb://workspaces/${wsID}/nodes/${child.id}`;
  const listed = await call("resources/list", {});
  expect(listed.resources).toEqual(
    expect.arrayContaining([expect.objectContaining({ uri }), expect.objectContaining({ uri: childURI })]),
  );
  const resource = await call("resources/read", { uri }, uri);
  expect(resource.contents).toEqual([expect.objectContaining({ uri, mimeType: "application/json" })]);
  expect(JSON.parse(resource.contents[0].text).content).toBe("Readable through MCP");

  await page.reload();
  await expect(page.getByTestId("connection-status")).toHaveText("Connected");
  await page.getByTestId(`sidebar-node-${node.id}`).locator("> div").first().click();
  const title = page.getByRole("textbox", { name: "Title", exact: true });
  const editor = page.getByTestId("wysiwyg-editor").locator(".ProseMirror");
  await expect(title).toHaveValue("Go Mode document");
  const update = async (newTitle: string, content: string) => {
    const changed = await call(
      "tools/call",
      {
        name: "node_update",
        arguments: { nodeId: node.id, title: newTitle, content },
      },
      "node_update",
    );
    expect(changed.isError).not.toBe(true);
    expect(changed.structuredContent.node).toEqual(expect.objectContaining({ title: newTitle, content }));
  };
  await update("Updated by Go Mode", "MCP changed the page currently visible");
  await expect(title).toHaveValue("Updated by Go Mode");
  await expect(editor).toContainText("MCP changed the page currently visible");

  await page.clock.install();
  await page.clock.pauseAt(new Date());
  await title.fill("Local draft");
  await update("Another MCP edit", "Keep the local draft until refreshed");
  await expect(
    page.getByText("This page changed elsewhere. Refresh to load it and discard your unsaved edits."),
  ).toBeVisible();
  await expect(title).toHaveValue("Local draft");
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(title).toHaveValue("Another MCP edit");
  await expect(editor).toContainText("Keep the local draft until refreshed");
});
