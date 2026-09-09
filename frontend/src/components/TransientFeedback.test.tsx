// Tests that transient operation feedback announces errors without entering layout flow.

import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@solidjs/testing-library';
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
});
