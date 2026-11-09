// Verifies isolated account identities and fail-fast authentication fixture responses.

import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import type { APIRequestContext } from "@playwright/test";
import { test, expect, registerUser, readDeviceLoginCode, readDeviceLoginProof } from "./helpers";

interface FixtureAPI {
  request: APIRequestContext;
  replies: { status: number; body: unknown }[];
  calls: unknown[];
}

const fixtureTest = test.extend<{ fixtureAPI: FixtureAPI }>({
  fixtureAPI: async ({ playwright }, use) => {
    const replies: FixtureAPI["replies"] = [];
    const calls: unknown[] = [];
    const server = createServer((req, res) => {
      void (async () => {
        let body = "";
        for await (const chunk of req) body += String(chunk);
        calls.push(body ? (JSON.parse(body) as unknown) : null);
        const reply = replies.shift();
        res.writeHead(reply?.status ?? 500, { "Content-Type": "application/json" });
        res.end(
          JSON.stringify(reply ? reply.body : { error: { code: "UNEXPECTED_REQUEST", message: "Unexpected request" } }),
        );
      })().catch((error: unknown) => res.destroy(error instanceof Error ? error : new Error(String(error))));
    });
    await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
    const request = await playwright.request.newContext({
      baseURL: `http://127.0.0.1:${(server.address() as AddressInfo).port}`,
    });
    try {
      await use({ request, replies, calls });
    } finally {
      await request.dispose();
      server.closeAllConnections();
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  },
});

fixtureTest("same-prefix registrations remain distinct when the clock does not advance", async ({ fixtureAPI }) => {
  fixtureAPI.replies.push({ status: 200, body: { token: "first" } }, { status: 200, body: { token: "second" } });
  const now = Date.now;
  Date.now = () => 1234567890000;
  try {
    const users = await Promise.all([
      registerUser(fixtureAPI.request, "same"),
      registerUser(fixtureAPI.request, "same"),
    ]);
    expect(new Set(users.map((user) => user.email)).size).toBe(2);
  } finally {
    Date.now = now;
  }
});

for (const body of [{}, { token: "" }, { token: 17 }, null]) {
  fixtureTest(`registration rejects unusable success payload ${JSON.stringify(body)}`, async ({ fixtureAPI }) => {
    fixtureAPI.replies.push({ status: 200, body });
    await expect(registerUser(fixtureAPI.request, "invalid")).rejects.toThrow(/registration.*token/i);
    expect(fixtureAPI.calls).toHaveLength(1);
  });
}

fixtureTest("registration retains its identity across a bounded rate-limit retry", async ({ fixtureAPI }) => {
  fixtureAPI.replies.push(
    {
      status: 429,
      body: { error: { code: "RATE_LIMITED", message: "Try later" }, details: { retry_after_seconds: 0 } },
    },
    { status: 200, body: { token: "registered" } },
  );
  await registerUser(fixtureAPI.request, "retry");
  expect(fixtureAPI.calls).toHaveLength(2);
  expect(fixtureAPI.calls[1]).toEqual(fixtureAPI.calls[0]);
});

fixtureTest(
  "registration stops after three rate-limited attempts without exposing the body",
  async ({ fixtureAPI }) => {
    for (let i = 0; i < 3; i++) {
      fixtureAPI.replies.push({
        status: 429,
        body: { error: { code: "RATE_LIMITED", message: "private body" }, details: { retry_after_seconds: 0 } },
      });
    }
    await expect(registerUser(fixtureAPI.request, "limited")).rejects.toMatchObject({
      message: "Registration failed (HTTP 429)",
    });
    expect(fixtureAPI.calls).toHaveLength(3);
    expect(fixtureAPI.calls[1]).toEqual(fixtureAPI.calls[0]);
    expect(fixtureAPI.calls[2]).toEqual(fixtureAPI.calls[0]);
  },
);

fixtureTest("registration rejects malformed retry delays without another request", async ({ fixtureAPI }) => {
  fixtureAPI.replies.push({
    status: 429,
    body: { error: { code: "RATE_LIMITED", message: "Try later" }, details: { retry_after_seconds: "private value" } },
  });
  await expect(registerUser(fixtureAPI.request, "delay")).rejects.toMatchObject({
    message: "Registration rate limit returned an invalid retry delay",
  });
  expect(fixtureAPI.calls).toHaveLength(1);
});

for (const [operation, read] of [
  ["Device login start", readDeviceLoginCode],
  ["Device login lookup", readDeviceLoginProof],
] as const) {
  for (const status of [403, 503]) {
    fixtureTest(`${operation} rejects HTTP ${status} before interpreting its payload`, async ({ fixtureAPI }) => {
      // Even a body containing all consumed fields cannot override failed HTTP status.
      fixtureAPI.replies.push({
        status,
        body: { user_code: "code", device_code: "private device", csrf_token: "private csrf" },
      });
      await expect(read(await fixtureAPI.request.post("/setup"))).rejects.toMatchObject({
        message: `${operation} failed (HTTP ${status})`,
      });
    });
  }
  fixtureTest(`${operation} rejects unusable success payloads without exposing credentials`, async ({ fixtureAPI }) => {
    fixtureAPI.replies.push({ status: 200, body: { user_code: "", device_code: "private device", csrf_token: 17 } });
    await expect(read(await fixtureAPI.request.post("/setup"))).rejects.toMatchObject({
      message:
        operation === "Device login start"
          ? "Device login start returned invalid user_code or device_code"
          : "Device login lookup returned invalid csrf_token",
    });
  });
}
