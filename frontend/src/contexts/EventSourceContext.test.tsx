// Tests SSE connection status across failure and recovery transitions.

import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen } from '@solidjs/testing-library';
import { EventSourceProvider, useEventSource } from './EventSourceContext';

vi.mock('./AuthContext', () => ({
  useAuth: () => ({
    user: () => ({ id: 'user-1', workspace_id: 'workspace-1' }),
    token: () => 'test-token',
  }),
}));

class MockEventSource {
  static instances: MockEventSource[] = [];
  onopen: ((event: Event) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  readonly close = vi.fn();

  constructor(readonly url: string) {
    MockEventSource.instances.push(this);
  }

  addEventListener(_type: string, _listener: unknown) {}

  open() {
    this.onopen?.(new Event('open'));
  }

  fail() {
    this.onerror?.(new Event('error'));
  }
}

function ConnectionStatus() {
  const { connected } = useEventSource();
  return <output data-testid="connection-status">{connected() ? 'Connected' : 'Reconnecting'}</output>;
}

afterEach(() => {
  cleanup();
  MockEventSource.instances = [];
  vi.unstubAllGlobals();
});

describe('EventSourceProvider', () => {
  it('reports reconnecting after a connection failure and connected after recovery', () => {
    vi.stubGlobal('EventSource', MockEventSource);

    render(() => (
      <EventSourceProvider>
        <ConnectionStatus />
      </EventSourceProvider>
    ));

    expect(MockEventSource.instances).toHaveLength(1);
    const eventSource = MockEventSource.instances[0];
    if (!eventSource) {
      throw new Error('EventSourceProvider did not create an event source');
    }
    expect(eventSource.url).toBe('/api/v1/workspaces/workspace-1/events?token=test-token');
    expect(screen.getByTestId('connection-status')).toHaveTextContent('Reconnecting');

    eventSource.open();
    expect(screen.getByTestId('connection-status')).toHaveTextContent('Connected');

    eventSource.fail();
    expect(screen.getByTestId('connection-status')).toHaveTextContent('Reconnecting');

    eventSource.open();
    expect(screen.getByTestId('connection-status')).toHaveTextContent('Connected');
  });
});
