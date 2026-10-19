// Exercises real voice overlay capture and scoped authorization across the gateway origin boundary.

import { test, expect, registerUser, createClient } from "./helpers";

test.use({ launchOptions: { args: ["--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream"] } });

for (const external of [true, false]) {
  test(`voice offers use scoped authorization with a ${external ? "remote" : "same-origin"} gateway`, async ({
    page,
    request,
    context,
    baseURL,
  }) => {
    const { token } = await registerUser(request, "gomode-voice");
    await context.grantPermissions(["microphone"]);
    let loginToken = token;
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
      const currentToken = await page.evaluate(() => localStorage.getItem("mddb_token"));
      expect(currentToken).not.toBeNull();
      loginToken = currentToken!;
      expect(route.request().headers().authorization).toBe(`Bearer ${currentToken}`);
      await route.fulfill({ json: service, headers: { "Cache-Control": "no-store" } });
    });
    await page.route(`${gateway}/api/voicegateway/v1/**`, async (route) => {
      const gatewayRequest = route.request();
      expect(gatewayRequest.headers().authorization).toBeUndefined();
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
        expect(gatewayRequest.postData()).not.toContain(loginToken);
        await route.fulfill({
          status: 503,
          headers,
          json: { error: { code: "unavailable", message: "Test gateway received voice offer" } },
        });
      } else {
        throw new Error(`Unexpected gateway request: ${gatewayRequest.url()}`);
      }
    });
    await page.goto(`/?token=${token}`);
    await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });
    const offer = page.waitForRequest(`${gateway}/api/voicegateway/v1/voice/rtc/offer`);
    await page.getByRole("button", { name: "Connect voice assistant", exact: true }).click();
    await offer;
    await expect(page.getByText("Test gateway received voice offer", { exact: true })).toBeVisible();
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
  await page.goto(`/?token=${token}`);
  await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });
  await page.getByRole("button", { name: "Connecter l’assistant vocal", exact: true }).click();
  await expect(page.getByText("Impossible d’autoriser la voix. Réessayez.", { exact: true })).toBeVisible();
  expect(offers).toBe(0);
  await expect(page.locator("body")).not.toContainText(token);
});
