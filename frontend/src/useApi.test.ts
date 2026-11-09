// Tests transport cancellation and UI write correlation across retries and independent MCP calls.

import { afterEach, describe, it } from "node:test";
import { expect, vi } from "@tests/expect";
import { createApi, createAuthFetch, isLocalWriteRequest, localWriteHeaders } from "./useApi";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("request cancellation", () => {
  it("cancels retry backoff before another transport attempt", async () => {
    vi.useFakeTimers();
    const controller = new AbortController();
    const fetch = vi.fn<typeof globalThis.fetch>(async () => new Response(null, { status: 503 }));
    vi.stubGlobal("fetch", fetch);
    let settled = false;
    const pending = createAuthFetch()("/retry", { signal: controller.signal }).catch((error: unknown) => {
      settled = true;
      return error;
    });
    await vi.advanceTimersByTimeAsync(0);
    expect(fetch).toHaveBeenCalledTimes(1);
    controller.abort();
    await vi.advanceTimersByTimeAsync(0);
    expect(settled).toBe(true);
    expect(await pending).toBe(controller.signal.reason);
    await vi.advanceTimersByTimeAsync(30_000);
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("does not admit a transport request after cancellation", async () => {
    const controller = new AbortController();
    controller.abort();
    const fetch = vi.fn<typeof globalThis.fetch>(async () => Response.json({}));
    vi.stubGlobal("fetch", fetch);
    await expect(createAuthFetch()("/canceled", { signal: controller.signal })).rejects.toBe(controller.signal.reason);
    expect(fetch).not.toHaveBeenCalled();
  });
});

describe("UI write request IDs", () => {
  it("tracks independent UI writes and preserves their IDs across retries and responses", async () => {
    vi.useFakeTimers();
    const seen: Headers[] = [];
    let attempted!: () => void;
    const firstAttempt = new Promise<void>((resolve) => {
      attempted = resolve;
    });
    const fetch = vi.fn<typeof globalThis.fetch>(async (_url, init) => {
      const headers = new Headers(init?.headers);
      seen.push(headers);
      expect(isLocalWriteRequest(headers.get("X-Request-ID") ?? undefined)).toBe(true);
      expect(headers.get("Authorization")).toBeNull();
      expect(init?.credentials).toBe("same-origin");
      if (seen.length === 1) {
        attempted();
        return new Response(null, { status: 503 });
      }
      return Response.json({ modified: seen.length });
    });
    vi.stubGlobal("fetch", fetch);
    const api = createApi();
    const pending = api
      .ws("workspace")
      .updatePage("node", { title: "One", content: "Body", expected_revision: "1".repeat(64) });
    await firstAttempt;
    await api.ws("workspace").updatePage("node", { title: "Two", content: "Body", expected_revision: "2".repeat(64) });
    await vi.advanceTimersByTimeAsync(1000);
    await pending;
    expect(seen).toHaveLength(3);
    const first = seen[0]!.get("X-Request-ID");
    expect(first).toBeTruthy();
    expect(seen[1]!.get("X-Request-ID")).not.toBe(first);
    expect(seen[2]!.get("X-Request-ID")).toBe(first);
    expect(isLocalWriteRequest(first ?? undefined)).toBe(true);
  });

  it("does not register reads or generic authenticated MCP writes as UI edits", async () => {
    const seen: Headers[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof globalThis.fetch>(async (_url, init) => {
        seen.push(new Headers(init?.headers));
        return Response.json({});
      }),
    );
    await createApi().ws("workspace").getNode("node");
    await createAuthFetch()("/api/v1/gomode/mcp", {
      method: "POST",
      headers: { "X-Request-ID": "gomode-operation" },
    });
    expect(seen[0]!.get("X-Request-ID")).toBeNull();
    expect(seen[1]!.get("X-Request-ID")).toBe("gomode-operation");
    expect(isLocalWriteRequest("gomode-operation")).toBe(false);
  });

  it("bounds retained writes while preserving recent upload and delayed-event IDs", () => {
    const first = localWriteHeaders({ Authorization: "Bearer upload" });
    expect(first.get("Authorization")).toBe("Bearer upload");
    const firstID = first.get("X-Request-ID")!;
    expect(isLocalWriteRequest(firstID)).toBe(true);
    let latest = firstID;
    for (let i = 0; i < 5000; i++) latest = localWriteHeaders(undefined).get("X-Request-ID")!;
    expect(isLocalWriteRequest(firstID)).toBe(false);
    expect(isLocalWriteRequest(latest)).toBe(true);
    expect(isLocalWriteRequest(undefined)).toBe(false);
  });
});
