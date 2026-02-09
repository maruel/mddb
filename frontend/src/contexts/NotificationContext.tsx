// Notification context providing notification state, polling, and push subscription management.

import {
  createContext,
  useContext,
  createSignal,
  onMount,
  onCleanup,
  type ParentComponent,
  type Accessor,
} from 'solid-js';
import type { NotificationDTO } from '@sdk/types.gen';
import type { APIClient } from '@sdk/api.gen';
import { registerServiceWorker } from '../notifications/sw-register';
import { subscribeToPush, unsubscribeFromPush } from '../notifications/push-manager';

const POLL_INTERVAL_MS = 60_000;
const PAGE_SIZE = 20;

interface NotificationContextValue {
  notifications: Accessor<NotificationDTO[]>;
  unreadCount: Accessor<number>;
  isLoading: Accessor<boolean>;
  pushEnabled: Accessor<boolean>;
  markAsRead: (id: string) => Promise<void>;
  markAllAsRead: () => Promise<void>;
  deleteNotification: (id: string) => Promise<void>;
  enablePush: () => Promise<boolean>;
  disablePush: () => Promise<void>;
  refresh: () => Promise<void>;
  loadMore: () => Promise<void>;
  hasMore: Accessor<boolean>;
}

const NotificationContext = createContext<NotificationContextValue>();

export const NotificationProvider: ParentComponent<{ api: Accessor<APIClient> }> = (props) => {
  const [notifications, setNotifications] = createSignal<NotificationDTO[]>([]);
  const [unreadCount, setUnreadCount] = createSignal(0);
  const [isLoading, setIsLoading] = createSignal(false);
  const [pushEnabled, setPushEnabled] = createSignal(false);
  const [hasMore, setHasMore] = createSignal(false);

  let swRegistration: ServiceWorkerRegistration | null = null;
  let pushSubscription: PushSubscription | null = null;
  let pollTimer: number | undefined;

  const fetchUnreadCount = async () => {
    try {
      const result = await props.api().notifications.getUnreadCount();
      setUnreadCount(result.count);
    } catch {
      // Silently ignore polling errors
    }
  };

  const fetchNotifications = async (offset = 0) => {
    setIsLoading(true);
    try {
      const result = await props.api().notifications.listNotifications({
        Limit: PAGE_SIZE,
        Offset: offset,
        UnreadOnly: false,
      });
      if (offset === 0) {
        setNotifications(result.notifications ?? []);
      } else {
        setNotifications((prev) => [...prev, ...(result.notifications ?? [])]);
      }
      setUnreadCount(result.unread_count);
      setHasMore((result.notifications?.length ?? 0) >= PAGE_SIZE);
    } catch {
      // Silently ignore
    } finally {
      setIsLoading(false);
    }
  };

  const refresh = async () => {
    await fetchNotifications(0);
  };

  const loadMore = async () => {
    await fetchNotifications(notifications().length);
  };

  const markAsRead = async (id: string) => {
    try {
      await props.api().notifications.markNotificationRead(id);
      setNotifications((prev) => prev.map((n) => (n.id === id ? { ...n, read: true } : n)));
      setUnreadCount((c) => Math.max(0, c - 1));
    } catch {
      // Silently ignore
    }
  };

  const markAllAsRead = async () => {
    try {
      await props.api().notifications.readAll.markAllNotificationsRead();
      setNotifications((prev) => prev.map((n) => ({ ...n, read: true })));
      setUnreadCount(0);
    } catch {
      // Silently ignore
    }
  };

  const deleteNotification = async (id: string) => {
    try {
      const target = notifications().find((item) => item.id === id);
      await props.api().notifications.deleteNotification(id);
      setNotifications((prev) => prev.filter((item) => item.id !== id));
      if (target && !target.read) {
        setUnreadCount((c) => Math.max(0, c - 1));
      }
    } catch {
      // Silently ignore
    }
  };

  const enablePush = async (): Promise<boolean> => {
    try {
      if (!swRegistration) {
        swRegistration = await registerServiceWorker();
      }
      if (!swRegistration) return false;
      const vapidResp = await props.api().notifications.vapidKey.getVAPIDPublicKey();
      pushSubscription = await subscribeToPush(swRegistration, vapidResp.public_key, props.api());
      if (pushSubscription) {
        setPushEnabled(true);
        return true;
      }
      return false;
    } catch {
      return false;
    }
  };

  const disablePush = async () => {
    try {
      if (pushSubscription) {
        await unsubscribeFromPush(pushSubscription, props.api());
        pushSubscription = null;
      }
      setPushEnabled(false);
    } catch {
      // Silently ignore
    }
  };

  onMount(async () => {
    // Initial fetch
    await fetchUnreadCount();

    // Check existing push subscription
    swRegistration = await registerServiceWorker();
    if (swRegistration) {
      const existing = await swRegistration.pushManager.getSubscription();
      if (existing) {
        pushSubscription = existing;
        setPushEnabled(true);
      }
    }

    // Start polling
    pollTimer = window.setInterval(fetchUnreadCount, POLL_INTERVAL_MS);
  });

  onCleanup(() => {
    if (pollTimer) {
      window.clearInterval(pollTimer);
    }
  });

  const value: NotificationContextValue = {
    notifications,
    unreadCount,
    isLoading,
    pushEnabled,
    markAsRead,
    markAllAsRead,
    deleteNotification,
    enablePush,
    disablePush,
    refresh,
    loadMore,
    hasMore,
  };

  return <NotificationContext.Provider value={value}>{props.children}</NotificationContext.Provider>;
};

export function useNotifications(): NotificationContextValue {
  const context = useContext(NotificationContext);
  if (!context) {
    throw new Error('useNotifications must be used within a NotificationProvider');
  }
  return context;
}
