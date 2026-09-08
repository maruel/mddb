// Tests for notification controls, keyboard dismissal, and trigger focus restoration.

import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@solidjs/testing-library';
import type { NotificationDTO } from '@sdk/types.gen';
import { I18nProvider } from '../i18n';
import type { NotificationContextValue } from '../contexts/NotificationContext';
import NotificationBell from './NotificationBell';

const notificationMocks = vi.hoisted(() => ({
  deleteNotification: vi.fn(),
  loadMore: vi.fn(),
  markAllAsRead: vi.fn(),
  markAsRead: vi.fn(),
  refresh: vi.fn(),
}));

vi.mock('../contexts/NotificationContext', () => ({
  useNotifications: (): NotificationContextValue => {
    const notifications: NotificationDTO[] = [
      {
        id: 'notification-1',
        type: 'page_edited',
        title: 'Test notification',
        body: 'A keyboard-operable notification action',
        read: false,
        created_at: Date.now(),
      },
    ];

    return {
      notifications: () => notifications,
      unreadCount: () => 1,
      isLoading: () => false,
      pushEnabled: () => false,
      markAsRead: async (id) => notificationMocks.markAsRead(id),
      markAllAsRead: async () => notificationMocks.markAllAsRead(),
      deleteNotification: async (id) => notificationMocks.deleteNotification(id),
      enablePush: async () => false,
      disablePush: async () => undefined,
      refresh: async () => notificationMocks.refresh(),
      loadMore: async () => notificationMocks.loadMore(),
      hasMore: () => false,
    };
  },
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('NotificationBell', () => {
  it('activates notification actions as buttons and restores trigger focus after Escape', async () => {
    render(() => (
      <I18nProvider>
        <NotificationBell />
      </I18nProvider>
    ));
    const trigger = screen.getByRole('button', { name: 'Notifications' });
    trigger.focus();
    fireEvent.click(trigger);

    await screen.findByRole('region', { name: 'Notifications' });
    const item = screen.getByRole('button', { name: /Test notification/ });
    fireEvent.click(item);
    expect(notificationMocks.markAsRead).toHaveBeenCalledWith('notification-1');

    fireEvent.keyDown(document, { key: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('region', { name: 'Notifications' })).toBeNull());
    expect(trigger).toHaveFocus();
  });
});
