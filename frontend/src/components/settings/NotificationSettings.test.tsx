// Tests browser push controls are unavailable in the Go Mode Android shell.

import { afterEach, describe, it } from "node:test";
import { expect } from "@tests/expect";
import { cleanup, render, screen } from "@solidjs/testing-library";
import { AuthContext, type AuthContextValue } from "../../contexts/AuthContext";
import { NotificationContext, type NotificationContextValue } from "../../contexts/NotificationContext";
import { I18nProvider } from "../../i18n";
import NotificationSettings from "./NotificationSettings";

const auth = {
  api: () => ({ getNotificationPrefs: async () => ({ defaults: {}, overrides: {} }) }),
} as unknown as AuthContextValue;

const notifications = {
  pushEnabled: () => false,
  enablePush: async () => false,
  disablePush: async () => undefined,
} as NotificationContextValue;

afterEach(() => {
  cleanup();
  delete window.goModeHost;
});

describe("NotificationSettings", () => {
  it("shows browser push only outside Go Mode", async () => {
    const renderSettings = () =>
      render(() => (
        <AuthContext.Provider value={auth}>
          <NotificationContext.Provider value={notifications}>
            <I18nProvider>
              <NotificationSettings />
            </I18nProvider>
          </NotificationContext.Provider>
        </AuthContext.Provider>
      ));

    renderSettings();
    expect(await screen.findByRole("button", { name: "Enable browser notifications" })).toBeTruthy();
    cleanup();

    window.goModeHost = {};
    renderSettings();
    expect(screen.queryByRole("button", { name: "Enable browser notifications" })).toBeNull();
  });
});
