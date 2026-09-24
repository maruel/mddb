// Tests mddb's Go Mode voice manifest selection and origin boundary.

import { describe, it } from "node:test";
import { expect } from "@tests/expect";
import { resolveVoiceEndpoints, voiceEndpointsFromResponse } from "./BrowserVoiceShell";

const origin = "https://mddb.example.com";

function manifest(url?: string) {
  return {
    service: "mddb",
    webShell: {
      toolGroups: [{ name: "workspace", endpoint: "/api/v1/gomode/mcp" }],
      voiceGateway: { url },
    },
  };
}

describe("resolveVoiceEndpoints", () => {
  it("uses the advertised same-origin gateway and workspace MCP endpoint", () => {
    expect(resolveVoiceEndpoints(manifest("/"), origin)).toEqual({
      gatewayURL: `${origin}/`,
      mcpEndpoint: `${origin}/api/v1/gomode/mcp`,
    });
    expect(resolveVoiceEndpoints(manifest(), origin)).toBeNull();
  });

  it("rejects another service or an external voice gateway", () => {
    expect(() => resolveVoiceEndpoints({ ...manifest("/"), service: "other" }, origin)).toThrow(
      "Unexpected Go Mode service",
    );
    expect(() => resolveVoiceEndpoints(manifest("https://other.example.com/"), origin)).toThrow("same-origin");
    expect(() =>
      resolveVoiceEndpoints(
        {
          ...manifest("/"),
          webShell: {
            ...manifest("/").webShell,
            toolGroups: [{ name: "workspace", endpoint: "https://other.example.com/mcp" }],
          },
        },
        origin,
      ),
    ).toThrow("same-origin");
  });
});

describe("voiceEndpointsFromResponse", () => {
  it("ignores missing manifests and SPA HTML fallbacks", async () => {
    const missing = new Response("Not found", { status: 404 });
    const fallback = new Response("<html>mddb</html>", { headers: { "content-type": "text/html" } });
    expect(await voiceEndpointsFromResponse(missing, origin)).toBeNull();
    expect(await voiceEndpointsFromResponse(fallback, origin)).toBeNull();
  });

  it("ignores malformed JSON from a broken manifest route", async () => {
    const malformed = new Response("{", { headers: { "content-type": "application/json" } });
    expect(await voiceEndpointsFromResponse(malformed, origin)).toBeNull();
  });

  it("rejects an advertised gateway with an unsafe origin", async () => {
    const response = Response.json(manifest("https://other.example.com/"));
    await expect(voiceEndpointsFromResponse(response, origin)).rejects.toThrow("same-origin");
  });
});
