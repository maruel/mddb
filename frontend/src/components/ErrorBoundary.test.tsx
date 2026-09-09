// Tests localized error-boundary recovery actions and diagnostic copying.

import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@solidjs/testing-library';
import { createSignal } from 'solid-js';
import { I18nProvider } from '../i18n';
import AppErrorBoundary from './ErrorBoundary';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function RecoveryHarness() {
  const [shouldFail, setShouldFail] = createSignal(true);

  function Content() {
    if (shouldFail()) {
      throw new Error('The test workspace failed');
    }
    return <p>Workspace recovered</p>;
  }

  return (
    <>
      <button type="button" onClick={() => setShouldFail(false)}>
        Permit recovery
      </button>
      <AppErrorBoundary>
        <Content />
      </AppErrorBoundary>
    </>
  );
}

describe('AppErrorBoundary', () => {
  it('copies diagnostic context and retries after the failure is resolved', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });

    render(() => (
      <I18nProvider>
        <RecoveryHarness />
      </I18nProvider>
    ));

    await screen.findByRole('alert');
    expect(screen.getByTestId('error-diagnostic')).toHaveTextContent('The test workspace failed');

    fireEvent.click(screen.getByRole('button', { name: 'Copy diagnostic details' }));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(expect.stringContaining('The test workspace failed')));
    await screen.findByText('Diagnostic details copied');

    fireEvent.click(screen.getByRole('button', { name: 'Permit recovery' }));
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    await screen.findByText('Workspace recovered');
  });
});
