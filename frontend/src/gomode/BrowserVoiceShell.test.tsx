// Tests mddb's Go Mode voice manifest selection and credential boundaries.

import { describe, it } from "node:test";
import { expect } from "@tests/expect";
import { resolveVoiceEndpoints, voiceEndpointsFromResponse } from "./BrowserVoiceShell";

const origin = "https://mddb.example.com";

function manifest(url?: string, tokenEndpoint?: string) {
  return {
    service: "mddb",
    webShell: {
      toolGroups: [{ name: "workspace", endpoint: "/api/v1/gomode/mcp" }],
      voiceGateway: { url, tokenEndpoint },
    },
  };
}

describe("resolveVoiceEndpoints", () => {
  it("keeps embedded bearer authentication without a token endpoint", () => {
    expect(resolveVoiceEndpoints(manifest("/"), origin)).toEqual({
      gatewayURL: `${origin}/`,
      mcpEndpoint: `${origin}/api/v1/gomode/mcp`,
      tokenEndpoint: null,
    });
    expect(resolveVoiceEndpoints(manifest(), origin)).toBeNull();
  });

  it("accepts an external gateway only with a same-origin token endpoint", () => {
    expect(resolveVoiceEndpoints(manifest("https://voice.example.com", "/api/v1/gomode/voice/token"), origin)).toEqual({
      gatewayURL: "https://voice.example.com/",
      mcpEndpoint: `${origin}/api/v1/gomode/mcp`,
      tokenEndpoint: `${origin}/api/v1/gomode/voice/token`,
    });
    expect(resolveVoiceEndpoints(manifest("/", "/api/v1/gomode/voice/token"), origin)?.tokenEndpoint).toBe(
      `${origin}/api/v1/gomode/voice/token`,
    );
    expect(() => resolveVoiceEndpoints(manifest("https://voice.example.com"), origin)).toThrow();
  });

  it("rejects unsafe gateway origins and paths", () => {
    for (const url of [
      "http://voice.example.com",
      "https://user:secret@voice.example.com",
      "https://@voice.example.com",
      "https://voice.example.com/path",
      "https://voice.example.com/path/..",
      "https://voice.example.com/.",
      "https://voice.example.com/%2e",
      "https://voice.example.com/?secret=x",
      "https://voice.example.com/#x",
      "javascript:alert(1)",
    ]) {
      expect(() => resolveVoiceEndpoints(manifest(url, "/token"), origin)).toThrow();
    }
    expect(resolveVoiceEndpoints(manifest("http://127.0.0.1:9000", "/token"), origin)?.gatewayURL).toBe(
      "http://127.0.0.1:9000/",
    );
  });

  it("rejects another service and unsafe mddb endpoints", () => {
    expect(() => resolveVoiceEndpoints({ ...manifest("/"), service: "other" }, origin)).toThrow(
      "Unexpected Go Mode service",
    );
    for (const endpoint of [
      "https://other.example.com/token",
      "https://u:p@mddb.example.com/token",
      "/token#fragment",
    ]) {
      expect(() => resolveVoiceEndpoints(manifest("/", endpoint), origin)).toThrow();
      const value = manifest("/");
      value.webShell.toolGroups[0]!.endpoint = endpoint;
      expect(() => resolveVoiceEndpoints(value, origin)).toThrow();
    }
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
});
