// Bell icon with unread badge and notification dropdown panel.

import { createEffect, createSignal, For, onCleanup, Show } from 'solid-js';
import { useNotifications } from '../contexts/NotificationContext';
import { useClickOutside } from '../composables/useClickOutside';
import { useI18n } from '../i18n';
import type { NotificationDTO } from '@sdk/types.gen';
import styles from './NotificationBell.module.css';

import NotificationsIcon from '@material-symbols/svg-400/outlined/notifications.svg?solid';
import CloseIcon from '@material-symbols/svg-400/outlined/close.svg?solid';

function relativeTime(
  ms: number,
  t: (key: string, params?: Record<string, string>) => string | undefined | null
): string {
  const diff = Date.now() - ms;
  const minutes = Math.floor(diff / 60_000);
  if (minutes < 1) return t('notifications.timeJustNow') || 'just now';
  if (minutes < 60) return t('notifications.timeMinutesAgo', { count: String(minutes) }) || `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return t('notifications.timeHoursAgo', { count: String(hours) }) || `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return t('notifications.timeDaysAgo', { count: String(days) }) || `${days}d ago`;
}

export default function NotificationBell() {
  const { t } = useI18n();
  const {
    notifications,
    unreadCount,
    isLoading,
    markAsRead,
    markAllAsRead,
    deleteNotification,
    refresh,
    loadMore,
    hasMore,
  } = useNotifications();
  const [isOpen, setIsOpen] = createSignal(false);
  let panelRef: HTMLDivElement | undefined;
  let triggerRef: HTMLButtonElement | undefined;

  const closePanel = () => {
    if (!isOpen()) return;
    setIsOpen(false);
    queueMicrotask(() => triggerRef?.focus());
  };

  useClickOutside(() => panelRef, closePanel);

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.preventDefault();
      closePanel();
    }
  };

  createEffect(() => {
    if (!isOpen()) return;
    document.addEventListener('keydown', handleKeyDown);
    onCleanup(() => document.removeEventListener('keydown', handleKeyDown));
  });

  const toggle = async () => {
    const opening = !isOpen();
    setIsOpen(opening);
    if (opening) {
      await refresh();
    }
  };

  const handleItemClick = async (n: NotificationDTO) => {
    if (!n.read) {
      await markAsRead(n.id);
    }
  };

  const handleDelete = async (e: MouseEvent, id: string) => {
    e.stopPropagation();
    await deleteNotification(id);
  };

  return (
    <div class={styles.bellWrapper} ref={(el) => (panelRef = el)}>
      <button
        ref={(el) => (triggerRef = el)}
        type="button"
        class={styles.bellButton}
        onClick={toggle}
        aria-label={t('notifications.title') || 'Notifications'}
        aria-expanded={isOpen()}
        aria-controls="notifications-panel"
      >
        <NotificationsIcon />
        <Show when={unreadCount() > 0}>
          <span class={styles.badge}>{unreadCount() > 99 ? '99+' : unreadCount()}</span>
        </Show>
      </button>

      <Show when={isOpen()}>
        <div id="notifications-panel" class={styles.panel} role="region" aria-label={t('notifications.title')}>
          <div class={styles.panelHeader}>
            <h3>{t('notifications.title')}</h3>
            <Show when={unreadCount() > 0}>
              <button type="button" class={styles.markAllButton} onClick={markAllAsRead}>
                {t('notifications.markAllRead')}
              </button>
            </Show>
          </div>

          <div class={styles.panelBody}>
            <Show
              when={notifications().length > 0}
              fallback={<div class={styles.empty}>{isLoading() ? t('common.loading') : t('notifications.empty')}</div>}
            >
              <For each={notifications()}>
                {(n) => (
                  <div class={`${styles.item} ${n.read ? '' : styles.unread}`}>
                    <button type="button" class={styles.itemButton} onClick={() => handleItemClick(n)}>
                      <div class={styles.itemContent}>
                        <div class={styles.itemTitle}>{n.title}</div>
                        <Show when={n.body}>
                          <div class={styles.itemBody}>{n.body}</div>
                        </Show>
                        <div class={styles.itemMeta}>
                          <span>{relativeTime(n.created_at, t)}</span>
                          <Show when={n.actor_name}>
                            <span class={styles.actor}>{n.actor_name}</span>
                          </Show>
                        </div>
                      </div>
                    </button>
                    <button
                      type="button"
                      class={styles.deleteButton}
                      onClick={(e) => handleDelete(e, n.id)}
                      aria-label={t('common.delete') || 'Delete'}
                    >
                      <CloseIcon />
                    </button>
                  </div>
                )}
              </For>
              <Show when={hasMore()}>
                <button type="button" class={styles.loadMoreButton} onClick={loadMore} disabled={isLoading()}>
                  {t('notifications.loadMore')}
                </button>
              </Show>
            </Show>
          </div>
        </div>
      </Show>
    </div>
  );
}
