// Internationalization provider and context hooks.

import { createContext, useContext, createSignal, createResource, type ParentComponent, type Accessor } from 'solid-js';
import * as i18n from '@solid-primitives/i18n';
import type { Dictionary, Locale } from './types';
import type { ErrorCode } from '@sdk/types.gen';

// Flatten the dictionary for dot-notation access
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type FlatDict = i18n.Flatten<Dictionary & Record<string, any>>;

// Dynamic import for dictionaries
async function fetchDictionary(locale: Locale): Promise<FlatDict> {
  const module = await import(`./dictionaries/${locale}.ts`);
  return i18n.flatten(module.dict) as FlatDict;
}

interface I18nContextValue {
  locale: Accessor<Locale>;
  setLocale: (locale: Locale) => void;
  t: i18n.NullableTranslator<FlatDict>;
  translateError: (code: ErrorCode | string) => string;
  ready: Accessor<boolean>;
}

const I18nContext = createContext<I18nContextValue>();

export const I18nProvider: ParentComponent<{ initialLocale?: Locale }> = (props) => {
  const [locale, setLocale] = createSignal<Locale>(props.initialLocale || 'en');
  const [dict] = createResource(locale, fetchDictionary);

  const t = i18n.translator(dict, i18n.resolveTemplate);

  // Expose whether translations are loaded
  const ready = () => dict.state === 'ready';

  const translateError = (code: ErrorCode | string): string => {
    // Try to find error message by code
    const key = `errors.${code}` as keyof FlatDict;
    const translated = t(key);
    if (translated && translated !== key) {
      return translated as string;
    }
    // Fallback to unknown error
    return (t('errors.unknown') as string) || 'An error occurred';
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
    throw new Error('useI18n must be used within I18nProvider');
  }
  return context;
}

export type { Locale, Dictionary };
