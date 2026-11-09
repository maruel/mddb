// Internationalization provider and context hooks.

import { createContext, useContext, createSignal, createResource, type ParentComponent, type Accessor } from "solid-js";
import * as i18n from "@solid-primitives/i18n";
import type { Dictionary, Locale } from "./types";
import type { ErrorCode } from "@sdk/types.gen";

// Flatten the dictionary for dot-notation access
type FlatDict = Record<string, string>;

// Dynamic import for dictionaries
async function fetchDictionary(locale: Locale): Promise<FlatDict> {
  const loaders = {
    en: () => import("./dictionaries/en.ts"),
    fr: () => import("./dictionaries/fr.ts"),
    de: () => import("./dictionaries/de.ts"),
    es: () => import("./dictionaries/es.ts"),
  };
  const module = await loaders[locale]();
  const entries = Object.entries(i18n.flatten({ ...module.dict }));
  const flat: FlatDict = {};
  for (const [key, value] of entries) {
    if (typeof value === "string") flat[key] = value;
  }
  return flat;
}

interface I18nContextValue {
  locale: Accessor<Locale>;
  setLocale: (locale: Locale) => void;
  t: (key: string, args?: Record<string, string | number>) => string;
  translateError: (code: ErrorCode | string) => string;
  ready: Accessor<boolean>;
}

const I18nContext = createContext<I18nContextValue>();

export const I18nProvider: ParentComponent<{ initialLocale?: Locale }> = (props) => {
  const [locale, setLocale] = createSignal<Locale>(props.initialLocale || "en");
  const [dict] = createResource(locale, fetchDictionary);

  const rawTranslator = i18n.translator(dict, i18n.resolveTemplate);

  // Wrap translator to ensure dict() is accessed on each call for proper reactivity.
  // This ensures Solid tracks dependencies correctly when locale changes.
  const t: I18nContextValue["t"] = (key, args) => {
    // Force dict access for reactivity tracking - this makes components
    // that call t() re-render when the dictionary changes
    dict();
    return rawTranslator(key, args) ?? "";
  };

  // Expose whether translations are loaded
  const ready = () => dict.state === "ready";

  const translateError = (code: ErrorCode | string): string => {
    // Try to find error message by code
    const key = `errors.${code}`;
    const translated = t(key);
    if (translated && translated !== key) {
      return translated;
    }
    // Fallback to unknown error
    return t("errors.unknown") || "An error occurred";
  };

  const value: I18nContextValue = {
    locale,
    setLocale,
    t,
    translateError,
    ready,
  };

  return <I18nContext.Provider value={value}>{props.children}</I18nContext.Provider>;
};

export function useI18n(): I18nContextValue {
  const context = useContext(I18nContext);
  if (!context) {
    throw new Error("useI18n must be used within I18nProvider");
  }
  return context;
}

export type { Locale, Dictionary };
