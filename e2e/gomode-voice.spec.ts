// Exercises voice settings, microphone permissions, and scoped authorization across gateway origins.

import { test, expect, registerUser, createClient, gotoWithSession } from "./helpers";
import type { VoiceChoicesResp } from "@maruel/gomode/sdk/voicegateway/v2/types.gen.ts";

// Individual tests grant capture explicitly; auto-grant UI would bypass denied permissions.
test.use({ launchOptions: { args: ["--use-fake-device-for-media-stream"] } });

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
    await page.addInitScript(() => {
      Object.defineProperty(speechSynthesis, "getVoices", {
        configurable: true,
        value: () => [
          { name: "US English", lang: "en-US", voiceURI: "english-us", default: true, localService: true },
          { name: "UK English", lang: "en-GB", voiceURI: "english-uk", default: false, localService: true },
          { name: "French", lang: "fr-FR", voiceURI: "french", default: false, localService: true },
        ],
      });
    });
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
    await page.route(`${gateway}/api/voicegateway/v2/**`, async (route) => {
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
          json: {
            choices: [
              {
                id: "fake",
                label: "Test voice",
                transports: ["webrtc"],
                composable: true,
                speechModes: [
                  { input: "gateway", output: "gateway" },
                  { input: "gateway", output: "client" },
                ],
              },
            ],
            recognitionModels: [{ id: "asr", label: "Test recognition" }],
            synthesisModels: [{ id: "tts", label: "Test synthesis" }],
            defaults: [{ transport: "webrtc", id: "fake" }],
          },
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
    if (!external) await page.setViewportSize({ width: 390, height: 844 });
    await page.getByRole("button", { name: "Voice settings", exact: true }).click();
    await page.getByRole("tab", { name: "Select parts", exact: true }).click();
    const recognition = page.getByRole("combobox", { name: "Recognition", exact: true });
    const output = page.getByRole("combobox", { name: "Speech", exact: true });
    await expect(recognition).toHaveValue("model:asr");
    await expect(page.getByRole("combobox", { name: "Language model", exact: true })).toHaveValue("fake");
    await expect(output).toHaveValue("model:tts");
    await output.selectOption("browser");
    await expect(recognition).toHaveValue("model:asr");
    const speakingVoice = page.getByRole("combobox", { name: "Speaking voice", exact: true });
    const language = page.getByRole("textbox", { name: "Voice language (e.g. en-US)", exact: true });
    await expect(speakingVoice.locator("option")).toHaveText([
      "Browser default",
      "UK English (en-GB)",
      "US English (en-US)",
    ]);
    await speakingVoice.selectOption("english-uk");
    await language.fill("en-AU");
    await language.press("Tab");
    await expect(speakingVoice).toHaveValue("english-uk");
    await language.fill("fr-CA");
    await language.press("Tab");
    await expect(speakingVoice.locator("option")).toHaveText([
      "Choose an available speaking voice",
      "Browser default",
      "French (fr-FR)",
    ]);
    await expect(speakingVoice).toHaveValue("english-uk");
    await speakingVoice.selectOption("french");
    await page.screenshot({ path: test.info().outputPath("voice-settings.png") });
    await output.selectOption("model:tts");
    await page.reload();
    await page.getByRole("button", { name: "Voice settings", exact: true }).click();
    await expect(page.getByRole("tab", { name: "Select parts", exact: true })).toHaveAttribute("aria-selected", "true");
    await expect(recognition).toHaveValue("model:asr");
    await expect(output).toHaveValue("model:tts");
    await page.getByRole("button", { name: "Close voice settings", exact: true }).click();
    const offer = page.waitForRequest(`${gateway}/api/voicegateway/v2/voice/rtc/offer`);
    await page.getByRole("button", { name: "Connect voice assistant", exact: true }).click();
    await offer;
    await expect(page.getByRole("region", { name: "Voice assistant", exact: true }).getByRole("alert")).toHaveText(
      "Voice gateway unavailable (HTTP 503).",
    );
  });
}

test("German voice setup keeps both tabs and stage choices usable on mobile", async ({ page, request, baseURL }) => {
  const { token } = await registerUser(request, "voice-setup-de");
  await createClient(request, token).updateUserSettings({ settings: { theme: "system", language: "de" } });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.route("**/.well-known/gomode.json", async (route) => {
    const response = await route.fetch();
    const manifest = await response.json();
    manifest.webShell.voiceGateway = { url: baseURL! };
    await route.fulfill({ response, json: manifest });
  });
  await page.route("**/api/voicegateway/v2/voice/choices", (route) =>
    route.fulfill({
      json: {
        choices: [
          { id: "native", label: "Gemini Live", transports: ["webrtc"] },
          {
            id: "parts",
            label: "Gemma 4 26B A4B",
            composable: true,
            transports: ["webrtc"],
            speechModes: [{ input: "gateway", output: "gateway" }],
          },
        ],
        recognitionModels: [{ id: "asr", label: "Voxtral Realtime" }],
        synthesisModels: [{ id: "tts", label: "Gemini TTS" }],
        defaults: [{ transport: "webrtc", id: "native" }],
      } satisfies VoiceChoicesResp,
    }),
  );
  await gotoWithSession(page, token);
  await page.getByRole("button", { name: "Spracheinstellungen", exact: true }).click();
  const allInOne = page.getByRole("tab", { name: "Komplettmodell", exact: true });
  const parts = page.getByRole("tab", { name: "Bausteine wählen", exact: true });
  await expect(allInOne).toHaveAttribute("aria-selected", "true");
  await expect(parts).toBeEnabled();
  await allInOne.focus();
  await allInOne.press("ArrowRight");
  await expect(parts).toBeFocused();
  await expect(parts).toHaveAttribute("aria-selected", "true");
  await expect(page.getByRole("combobox", { name: "Spracherkennung", exact: true })).toHaveValue("model:asr");
  await expect(page.getByRole("combobox", { name: "Sprachmodell", exact: true })).toHaveValue("parts");
  await expect(page.getByRole("combobox", { name: "Sprachausgabe", exact: true })).toHaveValue("model:tts");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("voice-parts-de-mobile.png") });
  await parts.press("Home");
  await expect(allInOne).toBeFocused();
  await expect(page.getByRole("combobox", { name: "Modell", exact: true })).toHaveValue("native");
  await page.screenshot({ path: test.info().outputPath("voice-all-in-one-de-mobile.png") });
});

test.describe("localized microphone permissions", () => {
  for (const capability of ["denied", "unavailable"] as const) {
    test(`localized microphone control explains ${capability} capture without starting voice`, async ({
      page,
      request,
      context,
      baseURL,
    }) => {
      const { token } = await registerUser(request, `voice-microphone-${capability}`);
      await createClient(request, token).updateUserSettings({ settings: { theme: "system", language: "de" } });
      await page.setViewportSize({ width: 390, height: 844 });
      if (capability === "unavailable") {
        await context.addInitScript(() => {
          Object.defineProperty(navigator, "mediaDevices", { configurable: true, value: undefined });
        });
      } else {
        const cdp = await context.newCDPSession(page);
        const { targetInfo } = await cdp.send("Target.getTargetInfo");
        if (!targetInfo.browserContextId) throw new Error("Missing isolated browser context");
        await cdp.send("Browser.setPermission", {
          permission: { name: "microphone" },
          setting: "denied",
          origin: baseURL!,
          browserContextId: targetInfo.browserContextId,
        });
      }
      const errors: string[] = [];
      page.on("pageerror", (error) => errors.push(error.message));
      let offers = 0;
      await page.route("**/.well-known/gomode.json", async (route) => {
        const response = await route.fetch();
        const manifest = await response.json();
        manifest.webShell.voiceGateway = { url: baseURL! };
        await route.fulfill({ response, json: manifest });
      });
      await page.route("**/api/voicegateway/v2/voice/choices", (route) =>
        route.fulfill({
          json: {
            choices: [{ id: "permission", label: "Teststimme", transports: ["webrtc"] }],
            defaults: [{ transport: "webrtc", id: "permission" }],
          } satisfies VoiceChoicesResp,
        }),
      );
      await page.route("**/api/voicegateway/v2/voice/rtc/offer", async (route) => {
        offers++;
        await route.abort();
      });
      await gotoWithSession(page, token);
      // First-login provisioning and its canonical roots precede voice controls.
      await expect(page.getByTestId("wysiwyg-editor")).toBeVisible({ timeout: 15000 });
      if (capability === "denied") {
        expect(
          await page.evaluate(
            async () => (await navigator.permissions.query({ name: "microphone" as PermissionName })).state,
          ),
        ).toBe("denied");
      }
      const control = page.getByRole("button", { name: "Mikrofon aktivieren", exact: true });
      const guidance =
        capability === "denied"
          ? "Erlauben Sie den Mikrofonzugriff in den Website-Berechtigungen Ihres Browsers."
          : "Das Mikrofon erfordert HTTPS oder einen unterstützten Browser.";
      await expect(control).toHaveAccessibleDescription(guidance);
      await control.click();
      await expect(control).toBeEnabled();
      await expect(page.getByText(guidance, { exact: true })).toBeVisible();
      await page.screenshot({ path: test.info().outputPath("microphone-help.png") });
      if (capability === "denied") {
        await context.grantPermissions(["microphone"], { origin: baseURL! });
        await expect(page.getByRole("button", { name: "Sprachassistent verbinden", exact: true })).toBeVisible();
      }
      expect(offers).toBe(0);
      expect(errors).toEqual([]);
    });
  }
});

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
        json: {
          choices: [{ id: "fake", label: "Test voice", transports: ["webrtc"] }],
          defaults: [{ transport: "webrtc", id: "fake" }],
        },
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
  await page.route("**/api/voicegateway/v2/voice/choices", (route) =>
    route.fulfill({
      json: {
        choices: [{ id: "fake", label: "Test voice", transports: ["webrtc"] }],
        defaults: [{ transport: "webrtc", id: "fake" }],
      },
    }),
  );
  const originating = page.waitForRequest(
    (req) => new URL(req.url()).pathname === "/api/v1/gomode/mcp" && req.method() === "POST",
  );
  await gotoWithSession(page, token);
  await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });
  // Workspace views reuse their screen event stream. Settings exercises the
  // independent MCP subscription and its captured session/workspace authority.
  await page.getByTestId("user-menu-button").click();
  await page.getByRole("menuitem", { name: "Profile", exact: true }).click();
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
  await page.route("**/api/voicegateway/v2/**", (route) => {
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
