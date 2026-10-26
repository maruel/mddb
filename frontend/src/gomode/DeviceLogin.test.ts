// Ensures every shared device-login state and action has a host translation.

import { it } from "node:test";
import { expect } from "@tests/expect";
import { defaultDeviceLoginMessages, defaultDeviceLoginRecoveryMessages } from "@maruel/gomode/web/DeviceLogin";
import { dict as de } from "../i18n/dictionaries/de";
import { dict as en } from "../i18n/dictionaries/en";
import { dict as es } from "../i18n/dictionaries/es";
import { dict as fr } from "../i18n/dictionaries/fr";

it("translates every device login label", () => {
  for (const [locale, dict] of Object.entries({ de, en, es, fr })) {
    const labels: Record<string, string> = dict.deviceLogin;
    for (const key of Object.keys({ ...defaultDeviceLoginMessages, ...defaultDeviceLoginRecoveryMessages }))
      expect(labels[key], `${locale} deviceLogin.${key}`).toBeTruthy();
  }
});
