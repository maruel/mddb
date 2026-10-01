// Tests that every Go Mode voice overlay label has a translation in each dictionary.

import { describe, it } from "node:test";
import { expect } from "@tests/expect";
import { defaultVoiceOverlayMessages } from "@maruel/gomode/web/VoiceOverlay";
import { dict as de } from "../i18n/dictionaries/de";
import { dict as en } from "../i18n/dictionaries/en";
import { dict as es } from "../i18n/dictionaries/es";
import { dict as fr } from "../i18n/dictionaries/fr";

describe("VoicePanel messages", () => {
  it("translates every gomode overlay label", () => {
    for (const [locale, dict] of Object.entries({ de, en, es, fr })) {
      const voice: Record<string, string> = dict.voice;
      for (const key of Object.keys(defaultVoiceOverlayMessages)) {
        expect(voice[key], `${locale} voice.${key}`).toBeTruthy();
      }
    }
  });
});
