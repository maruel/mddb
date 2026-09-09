// Tests that transient operation feedback announces errors without entering layout flow.

import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@solidjs/testing-library';
import { createSignal } from 'solid-js';
import { I18nProvider } from '../i18n';
import { TransientFeedback } from './TransientFeedback';

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe('TransientFeedback', () => {
  it('announces the failure and can be dismissed without changing surrounding content', async () => {
    const onDismiss = vi.fn();
    render(() => (
      <I18nProvider>
        <main>Workspace content</main>
        <TransientFeedback message="Failed to save" onDismiss={onDismiss} />
      </I18nProvider>
    ));

    const feedback = await screen.findByRole('alert');
    expect(feedback).toHaveAttribute('data-testid', 'workspace-feedback');
    expect(screen.getByText('Workspace content')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Close' }));
    expect(onDismiss).toHaveBeenCalledOnce();
  });

  it('restarts its dismissal timer when a new failure replaces the message', async () => {
    vi.useFakeTimers();
    const onDismiss = vi.fn();

    function FeedbackHarness() {
      const [message, setMessage] = createSignal('Failed to load');
      return (
        <I18nProvider>
          <button type="button" onClick={() => setMessage('Failed to save')}>
            Replace failure
          </button>
          <TransientFeedback message={message()} onDismiss={onDismiss} />
        </I18nProvider>
      );
    }

    render(() => <FeedbackHarness />);
    await vi.advanceTimersByTimeAsync(7_000);
    fireEvent.click(screen.getByRole('button', { name: 'Replace failure' }));

    await vi.advanceTimersByTimeAsync(1_000);
    expect(onDismiss).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(7_000);
    expect(onDismiss).toHaveBeenCalledOnce();
  });
});
