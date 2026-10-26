// Provides session-owned notifications and browser push registration.

import {
  createContext,
  useContext,
  createSignal,
  onMount,
  onCleanup,
  type ParentComponent,
  type Accessor,
} from "solid-js";
import type { NotificationDTO } from "@sdk/types.gen";
import type { APIClient } from "@sdk/api.gen";
import { registerServiceWorker } from "../notifications/sw-register";
import { subscribeToPush, unsubscribeFromPush } from "../notifications/push-manager";
import { isGoModeHost } from "../gomode/host";

const POLL_INTERVAL_MS = 60_000;
const PAGE_SIZE = 20;

export interface NotificationContextValue {
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

// Exported so tests can provide canned notification state without mocking the module.
export const NotificationContext = createContext<NotificationContextValue>();

export const NotificationProvider: ParentComponent<{ api: Accessor<APIClient> }> = (props) => {
  const [notifications, setNotifications] = createSignal<NotificationDTO[]>([]);
  const [unreadCount, setUnreadCount] = createSignal(0);
  const [isLoading, setIsLoading] = createSignal(false);
  const [pushEnabled, setPushEnabled] = createSignal(false);
  const [hasMore, setHasMore] = createSignal(false);

  let swRegistration: ServiceWorkerRegistration | null = null;
  let pushSubscription: PushSubscription | null = null;
  let pollTimer: number | undefined;
  let disposed = false;
  const current = (client: APIClient) => !disposed && props.api() === client;

  const fetchUnreadCount = async () => {
    const client = props.api();
    try {
      const result = await client.getUnreadCount();
      if (!current(client)) return;
      setUnreadCount(result.count);
    } catch {
      // Silently ignore polling errors
    }
  };

  const fetchNotifications = async (offset = 0) => {
    const client = props.api();
    setIsLoading(true);
    try {
      const result = await client.listNotifications({
        Limit: PAGE_SIZE,
        Offset: offset,
        UnreadOnly: false,
      });
      if (!current(client)) return;
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
      if (current(client)) setIsLoading(false);
    }
  };

  const refresh = async () => {
    await fetchNotifications(0);
  };

  const loadMore = async () => {
    await fetchNotifications(notifications().length);
  };

  const markAsRead = async (id: string) => {
    const client = props.api();
    try {
      await client.markNotificationRead(id);
      if (!current(client)) return;
      setNotifications((prev) => prev.map((n) => (n.id === id ? { ...n, read: true } : n)));
      setUnreadCount((c) => Math.max(0, c - 1));
    } catch {
      // Silently ignore
    }
  };

  const markAllAsRead = async () => {
    const client = props.api();
    try {
      await client.markAllNotificationsRead();
      if (!current(client)) return;
      setNotifications((prev) => prev.map((n) => ({ ...n, read: true })));
      setUnreadCount(0);
    } catch {
      // Silently ignore
    }
  };

  const deleteNotification = async (id: string) => {
    const client = props.api();
    try {
      const target = notifications().find((item) => item.id === id);
      await client.deleteNotification(id);
      if (!current(client)) return;
      setNotifications((prev) => prev.filter((item) => item.id !== id));
      if (target && !target.read) {
        setUnreadCount((c) => Math.max(0, c - 1));
      }
    } catch {
      // Silently ignore
    }
  };

  const enablePush = async (): Promise<boolean> => {
    const client = props.api();
    if (isGoModeHost()) return false;
    try {
      if (!swRegistration) {
        swRegistration = await registerServiceWorker();
      }
      if (!current(client) || !swRegistration) return false;
      const vapidResp = await client.getVAPIDPublicKey();
      if (!current(client)) return false;
      const subscription = await subscribeToPush(swRegistration, vapidResp.public_key, client);
      if (!current(client)) return false;
      pushSubscription = subscription;
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
    const client = props.api();
    try {
      if (pushSubscription) {
        await unsubscribeFromPush(pushSubscription, client);
        if (!current(client)) return;
        pushSubscription = null;
      }
      setPushEnabled(false);
    } catch {
      // Silently ignore
    }
  };

  onMount(async () => {
    const client = props.api();
    // Initial fetch
    await fetchUnreadCount();
    if (!current(client)) return;

    // Check existing push subscription
    if (!isGoModeHost()) {
      swRegistration = await registerServiceWorker();
      if (!current(client)) return;
      if (swRegistration) {
        const existing = await swRegistration.pushManager.getSubscription();
        if (!current(client)) return;
        if (existing) {
          pushSubscription = existing;
          setPushEnabled(true);
        }
      }
    }

    // Start polling
    pollTimer = window.setInterval(fetchUnreadCount, POLL_INTERVAL_MS);
  });

  onCleanup(() => {
    disposed = true;
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
    throw new Error("useNotifications must be used within a NotificationProvider");
  }
  return context;
}
