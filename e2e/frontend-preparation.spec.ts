// Verifies built mddb publishes explicit zero-tool readiness only for its validated identity.

import { expect, gotoWithSession, registerUser, test } from "./helpers";

function gate() {
  let resolve: () => void = () => {
    throw new Error("Gate is not initialized");
  };
  const promise = new Promise<void>((complete) => {
    resolve = complete;
  });
  return { promise, resolve };
}

test("native preparation waits for cookie identity and revokes on logout", async ({ page, request }) => {
  const { token } = await registerUser(request, "frontend-ready");
  const identity = gate();
  const requested = gate();
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.addInitScript(() => {
    const messages: Array<{ ready: boolean; tools: unknown[] }> = [];
    const bridge: { onmessage?: ((event: { data: string }) => void) | null; postMessage(message: string): void } = {
      postMessage(message) {
        const auth = JSON.parse(message) as Record<string, unknown>;
        if (auth.authChanged === true && auth.nativeAccess === true) {
          bridge.onmessage?.({ data: JSON.stringify({ ...auth, frontendToken: "native-identity" }) });
        }
      },
    };
    Object.assign(window, {
      goModeHost: {},
      goModeFrontendDocument: Promise.resolve("native-document"),
      gomodeAuth: bridge,
      gomodeFrontendReadiness: { postMessage: (message: string) => messages.push(JSON.parse(message)) },
      preparationMessages: messages,
    });
  });
  await page.route("**/api/v1/auth/me", async (route) => {
    requested.resolve();
    await identity.promise;
    await route.continue();
  });
  await gotoWithSession(page, token);
  await requested.promise;
  const readiness = () =>
    page.evaluate(
      () =>
        (window as unknown as { preparationMessages: Array<{ ready: boolean; tools: unknown[] }> }).preparationMessages,
    );
  expect(await readiness()).toEqual([]);
  identity.resolve();
  await expect
    .poll(async () => (await readiness()).slice(-1)[0])
    .toEqual({
      document: "native-document",
      token: "native-identity",
      ready: true,
      tools: [],
    });
  await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });
  await page.getByTestId("user-menu-button").click();
  await page.getByRole("menuitem", { name: "Logout" }).click();
  await expect
    .poll(async () => (await readiness()).slice(-1)[0])
    .toEqual({
      document: "native-document",
      token: "native-identity",
      ready: false,
      tools: [],
    });
  expect(errors).toEqual([]);
});
