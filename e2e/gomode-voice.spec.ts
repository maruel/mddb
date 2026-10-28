// Exercises real voice overlay capture and scoped authorization across the gateway origin boundary.

import { test, expect, registerUser, createClient, gotoWithSession } from "./helpers";

test.use({ launchOptions: { args: ["--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream"] } });

for (const external of [true, false]) {
  test(`voice offers use scoped authorization with a ${external ? "remote" : "same-origin"} gateway`, async ({
    page,
    request,
    context,
    baseURL,
  }) => {
    const { token } = await registerUser(request, "gomode-voice");
    const expectedSession = (await createClient(request, token).getMe()).session_id;
    await context.grantPermissions(["microphone"]);
    const gateway = external ? "https://voice.example.com" : baseURL!;
    const service = { kind: "mddb", instanceID: "test-main", baseURL: baseURL!, token: "short-lived-voice-token" };
    await page.route("**/.well-known/gomode.json", async (route) => {
      const response = await route.fetch();
      const manifest = await response.json();
      manifest.webShell.voiceGateway = { url: gateway, tokenEndpoint: "/api/v1/gomode/voice/token" };
      await route.fulfill({ response, json: manifest });
    });
    await page.route("**/api/v1/gomode/voice/token", async (route) => {
      expect(route.request().method()).toBe("POST");
      expect(await page.evaluate(() => localStorage.getItem("mddb_token"))).toBeNull();
      expect(route.request().headers().authorization).toBeUndefined();
      expect(route.request().headers().cookie).toContain("__Host-mddb_session=");
      expect(route.request().headers()["x-mddb-session"]).toBe(expectedSession);
      await route.fulfill({ json: service, headers: { "Cache-Control": "no-store" } });
    });
    await page.route(`${gateway}/api/voicegateway/v1/**`, async (route) => {
      const gatewayRequest = route.request();
      expect(gatewayRequest.headers().authorization).toBeUndefined();
      expect(gatewayRequest.headers()["x-mddb-session"]).toBe(external ? undefined : expectedSession);
      const headers = {
        "Access-Control-Allow-Origin": baseURL!,
        "Access-Control-Allow-Headers": "Content-Type",
        "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
      };
      if (gatewayRequest.method() === "OPTIONS") {
        await route.fulfill({ status: 204, headers });
      } else if (gatewayRequest.url().endsWith("/voice/choices")) {
        await route.fulfill({
          headers,
          json: { choices: [{ id: "fake", label: "Test voice", transports: ["webrtc"] }], defaultChoiceID: "fake" },
        });
      } else if (gatewayRequest.url().endsWith("/voice/rtc/offer")) {
        const body = gatewayRequest.postDataJSON();
        expect(body.service).toEqual(service);
        expect(body.choiceID).toBe("fake");
        expect(body.sdp).toContain("m=audio");
        expect(gatewayRequest.postData()).not.toContain(token);
        await route.fulfill({
          status: 503,
          headers,
          json: { error: { code: "unavailable", message: "Test gateway received voice offer" } },
        });
      } else {
        throw new Error(`Unexpected gateway request: ${gatewayRequest.url()}`);
      }
    });
    await gotoWithSession(page, token);
    await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });
    const offer = page.waitForRequest(`${gateway}/api/voicegateway/v1/voice/rtc/offer`);
    await page.getByRole("button", { name: "Connect voice assistant", exact: true }).click();
    await offer;
    await expect(page.getByRole("region", { name: "Voice assistant", exact: true }).getByRole("alert")).toHaveText(
      "Voice gateway unavailable (HTTP 503).",
    );
  });
}

test("voice token failure stops the offer without forwarding login credentials", async ({ page, request, context }) => {
  const { token } = await registerUser(request, "gomode-voice-denied");
  await createClient(request, token).updateUserSettings({ settings: { theme: "system", language: "fr" } });
  await context.grantPermissions(["microphone"]);
  await page.route("**/.well-known/gomode.json", async (route) => {
    const response = await route.fetch();
    const manifest = await response.json();
    manifest.webShell.voiceGateway = { url: "https://voice.example.com", tokenEndpoint: "/api/v1/gomode/voice/token" };
    await route.fulfill({ response, json: manifest });
  });
  await page.route("**/api/v1/gomode/voice/token", (route) =>
    route.fulfill({ status: 401, body: `Private details: ${token}` }),
  );
  let offers = 0;
  await page.route("https://voice.example.com/**", async (route) => {
    expect(route.request().headers().authorization).toBeUndefined();
    if (route.request().url().endsWith("/voice/choices")) {
      await route.fulfill({
        headers: { "Access-Control-Allow-Origin": "*" },
        json: { choices: [{ id: "fake", label: "Test voice", transports: ["webrtc"] }], defaultChoiceID: "fake" },
      });
    } else {
      offers++;
      await route.abort();
    }
  });
  await gotoWithSession(page, token);
  await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });
  await page.getByRole("button", { name: "Connecter l’assistant vocal", exact: true }).click();
  await expect(page.getByText("Impossible d’autoriser la voix. Réessayez.", { exact: true })).toBeVisible();
  expect(offers).toBe(0);
  await expect(page.locator("body")).not.toContainText(token);
});

test("a late retry of browser MCP work cannot use replacement-cookie authority", async ({
  page,
  request,
  context,
  baseURL,
}) => {
  const { token, email } = await registerUser(request, "voice-session-pin");
  const original = await createClient(request, token).getMe();
  const replacement = await createClient(request).login({ email, password: "testpassword123" });
  await context.grantPermissions(["microphone"]);
  await page.route("**/.well-known/gomode.json", async (route) => {
    const response = await route.fetch();
    const manifest = await response.json();
    manifest.webShell.voiceGateway = { url: baseURL! };
    await route.fulfill({ response, json: manifest });
  });
  await page.route("**/api/voicegateway/v1/voice/choices", (route) =>
    route.fulfill({
      json: { choices: [{ id: "fake", label: "Test voice", transports: ["webrtc"] }], defaultChoiceID: "fake" },
    }),
  );
  const originating = page.waitForRequest(
    (req) => new URL(req.url()).pathname === "/api/v1/gomode/mcp" && req.method() === "POST",
  );
  await gotoWithSession(page, token);
  await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });
  const queued = await originating;
  const cookie = (await context.cookies()).find((entry) => entry.name === "__Host-mddb_session");
  if (!cookie) throw new Error("Missing host cookie");
  // Replay the actual originating request after browser credentials change.
  // Its public pin must remain A while fetch selects the current cookie B.
  await context.addCookies([{ ...cookie, value: replacement.token }]);
  expect(new URL(queued.url()).searchParams.get("session_id")).toBe(original.session_id);
  const headers = Object.fromEntries(
    Object.entries(queued.headers()).filter(([key]) => key.startsWith("mcp-") || key === "content-type"),
  );
  const status = await page.evaluate(
    async (input) =>
      (await fetch(input.url, { method: "POST", headers: input.headers, body: input.body, credentials: "same-origin" }))
        .status,
    { url: queued.url(), headers, body: queued.postData() },
  );
  expect(status).toBe(401);
  expect((await createClient(request, replacement.token).getMe()).session_id).not.toBe(original.session_id);
});

function gate() {
  let resolve!: () => void;
  const promise = new Promise<void>((complete) => {
    resolve = complete;
  });
  return { promise, resolve };
}

test("logout aborts pending voice discovery and a late manifest cannot mount controls", async ({
  page,
  request,
  baseURL,
}) => {
  const { token } = await registerUser(request, "voice-discovery-logout");
  const discovery = gate();
  const release = gate();
  const finished = gate();
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  let gatewayRequests = 0;
  await page.route("**/api/voicegateway/v1/**", (route) => {
    gatewayRequests++;
    return route.abort();
  });
  await page.route("**/.well-known/gomode.json", async (route) => {
    const response = await route.fetch();
    const manifest = await response.json();
    manifest.webShell.voiceGateway = { url: baseURL! };
    discovery.resolve();
    await release.promise;
    await route.fulfill({ response, json: manifest });
    finished.resolve();
  });
  await gotoWithSession(page, token);
  await discovery.promise;
  await expect(page.getByTestId("user-menu-button")).toBeVisible();
  const aborted = page.waitForEvent("requestfailed", (req) => req.url().endsWith("/.well-known/gomode.json"));
  await page.getByTestId("user-menu-button").click();
  await page.getByRole("menuitem", { name: "Logout" }).click();
  await aborted;
  await expect(page.getByRole("heading", { name: "Login to mddb" })).toBeVisible();
  release.resolve();
  await finished.promise;
  await expect(page.getByRole("region", { name: "Voice assistant", exact: true })).toHaveCount(0);
  expect(gatewayRequests).toBe(0);
  expect(errors).toEqual([]);
});
