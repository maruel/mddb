// Tests scoped voice authorization exchange without exposing mddb credentials or server error bodies.

import { afterEach, describe, it } from "node:test";
import { expect, vi } from "@tests/expect";
import { voiceAuthorization } from "./voiceAuthorization";

const originalFetch = globalThis.fetch;
const endpoint = "https://mddb.example.com/api/v1/gomode/voice/token";
const failureMessage = "Sprachzugriff fehlgeschlagen";
const service = { kind: "mddb", instanceID: "main", baseURL: "https://mddb.example.com", token: "short-lived" };
afterEach(() => {
  globalThis.fetch = originalFetch;
});

describe("voiceAuthorization", () => {
  it("exchanges the current login token with an uncacheable same-origin POST", async () => {
    const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValue(Response.json(service));
    globalThis.fetch = fetch;
    expect(await voiceAuthorization(endpoint, "login-secret", failureMessage)).toEqual(service);
    expect(fetch).toHaveBeenCalledExactlyOnceWith(endpoint, {
      method: "POST",
      headers: { Authorization: "Bearer login-secret" },
      cache: "no-store",
      credentials: "same-origin",
      redirect: "error",
    });
  });

  it("fails before making a request when logged out", async () => {
    const fetch = vi.fn<typeof globalThis.fetch>();
    globalThis.fetch = fetch;
    await expect(voiceAuthorization(endpoint, null, failureMessage)).rejects.toThrow("Sprachzugriff fehlgeschlagen");
    expect(fetch).not.toHaveBeenCalled();
  });

  it("reports failures without including potentially secret response text", async () => {
    globalThis.fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValue(new Response("login-secret", { status: 401 }));
    await expect(voiceAuthorization(endpoint, "login-secret", failureMessage)).rejects.toThrow(
      /^Sprachzugriff fehlgeschlagen$/,
    );
  });

  it("uses the translated message for network failures without exposing request details", async () => {
    globalThis.fetch = vi.fn<typeof globalThis.fetch>().mockRejectedValue(new Error("login-secret"));
    await expect(voiceAuthorization(endpoint, "login-secret", failureMessage)).rejects.toThrow(
      /^Sprachzugriff fehlgeschlagen$/,
    );
  });

  it("rejects malformed authorization and unsafe issuer origins", async () => {
    for (const value of [
      null,
      [],
      { ...service, kind: "other" },
      { ...service, instanceID: "" },
      { ...service, token: "" },
      { ...service, baseURL: "https://u:secret@mddb.example.com" },
      { ...service, baseURL: "http://public.example.com" },
      { ...service, baseURL: "https://mddb.example.com/path" },
      { ...service, baseURL: "/" },
    ]) {
      globalThis.fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValue(Response.json(value));
      await expect(voiceAuthorization(endpoint, "login-secret", failureMessage)).rejects.toThrow(
        /^Sprachzugriff fehlgeschlagen$/,
      );
    }
    globalThis.fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValue(new Response("secret-invalid-json"));
    await expect(voiceAuthorization(endpoint, "login-secret", failureMessage)).rejects.toThrow(
      /^Sprachzugriff fehlgeschlagen$/,
    );
  });
});
