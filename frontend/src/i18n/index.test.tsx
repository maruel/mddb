import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, cleanup } from '@solidjs/testing-library';
import { onMount } from 'solid-js';
import { I18nProvider, useI18n } from './index';

// Helper component that uses the i18n hook
function TestConsumer(props: { onReady?: (ctx: ReturnType<typeof useI18n>) => void }) {
  const ctx = useI18n();
  onMount(() => props.onReady?.(ctx));
  return (
    <div>
      <span data-testid="locale">{ctx.locale()}</span>
      <span data-testid="translated">{ctx.t('common.loading')}</span>
      <button data-testid="change-locale" onClick={() => ctx.setLocale('fr')}>
        Change to FR
      </button>
    </div>
  );
}

describe('I18nProvider', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    cleanup();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('provides default locale as en', async () => {
    const { unmount } = render(() => (
      <I18nProvider>
        <TestConsumer />
      </I18nProvider>
    ));

    expect(screen.getByTestId('locale').textContent).toBe('en');
    unmount();
  });

  it('accepts initialLocale prop', async () => {
    const { unmount } = render(() => (
      <I18nProvider initialLocale="de">
        <TestConsumer />
      </I18nProvider>
    ));

    expect(screen.getByTestId('locale').textContent).toBe('de');
    unmount();
  });

  it('loads dictionary and translates keys', async () => {
    const { unmount } = render(() => (
      <I18nProvider>
        <TestConsumer />
      </I18nProvider>
    ));

    // Wait for dictionary to load
    await waitFor(() => {
      const text = screen.getByTestId('translated').textContent;
      expect(text).toBe('Loading...');
    });
    unmount();
  });

  it('allows changing locale', async () => {
    const { unmount } = render(() => (
      <I18nProvider>
        <TestConsumer />
      </I18nProvider>
    ));

    const button = screen.getByTestId('change-locale');
    button.click();

    await waitFor(() => {
      expect(screen.getByTestId('locale').textContent).toBe('fr');
    });
    unmount();
  });

  it('updates translations when locale changes', async () => {
    const { unmount } = render(() => (
      <I18nProvider>
        <TestConsumer />
      </I18nProvider>
    ));

    // Wait for initial English dictionary to load
    await waitFor(() => {
      expect(screen.getByTestId('translated').textContent).toBe('Loading...');
    });

    // Click button to change to French
    const button = screen.getByTestId('change-locale');
    button.click();

    // Wait for French dictionary to load and translation to update
    await waitFor(() => {
      // French translation for 'common.loading' is 'Chargement...'
      expect(screen.getByTestId('translated').textContent).toBe('Chargement...');
    });
    unmount();
  });
});

describe('useI18n', () => {
  beforeEach(() => {
    cleanup();
  });

  afterEach(() => {
    cleanup();
  });

  it('throws error when used outside I18nProvider', () => {
    // This should throw when trying to use the hook outside provider
    expect(() => {
      render(() => <TestConsumer />);
    }).toThrow('useI18n must be used within I18nProvider');
  });
});

describe('translateError', () => {
  beforeEach(() => {
    cleanup();
  });

  afterEach(() => {
    cleanup();
  });

  it('translates known error codes', async () => {
    let translateError: ((code: string) => string) | undefined;

    const { unmount } = render(() => (
      <I18nProvider>
        <TestConsumer
          onReady={(ctx) => {
            translateError = ctx.translateError;
          }}
        />
      </I18nProvider>
    ));

    // Wait for dictionary to load
    await waitFor(() => {
      expect(screen.getByTestId('translated').textContent).toBe('Loading...');
    });

    if (translateError) {
      const result = translateError('NOT_FOUND');
      expect(result).toBe('The requested resource was not found');
    }
    unmount();
  });

  it('returns fallback for unknown error codes', async () => {
    let translateError: ((code: string) => string) | undefined;

    const { unmount } = render(() => (
      <I18nProvider>
        <TestConsumer
          onReady={(ctx) => {
            translateError = ctx.translateError;
          }}
        />
      </I18nProvider>
    ));

    // Wait for dictionary to load
    await waitFor(() => {
      expect(screen.getByTestId('translated').textContent).toBe('Loading...');
    });

    if (translateError) {
      const result = translateError('UNKNOWN_ERROR_CODE_XYZ');
      // Should fall back to 'unknown' translation or default message
      expect(result).toMatch(/error|occurred/i);
    }
    unmount();
  });
});
