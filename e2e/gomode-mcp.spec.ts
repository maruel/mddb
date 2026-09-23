// Verifies Go Mode discovery and authenticated MCP reads against a real workspace.

import { test, expect, registerUser, getWorkspaceId, createClient } from "./helpers";

const protocolVersion = "2026-07-28";

test("Go Mode discovers mddb and reads only the active workspace", async ({ page, request }) => {
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
    const response = await request.post("/api/v1/gomode/mcp", {
      headers: {
        Authorization: `Bearer ${token}`,
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
    });
    expect(response.status()).toBe(200);
    const body = await response.json();
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
});
