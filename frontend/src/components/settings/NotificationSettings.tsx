// Notification preferences panel for managing per-type channel preferences and push subscription.

import { createSignal, createEffect, Show, For } from 'solid-js';
import type { ChannelSetDTO, NotificationPrefsDTO } from '@sdk/types.gen';
import { useAuth } from '../../contexts';
import { useNotifications } from '../../contexts/NotificationContext';
import { useI18n } from '../../i18n';
import styles from './NotificationSettings.module.css';

const NOTIFICATION_TYPES = [
  'org_invite',
  'ws_invite',
  'member_joined',
  'member_removed',
  'page_mention',
  'page_edited',
] as const;

const TYPE_LABEL_KEYS: Record<string, string> = {
  org_invite: 'notifications.typeOrgInvite',
  ws_invite: 'notifications.typeWsInvite',
  member_joined: 'notifications.typeMemberJoined',
  member_removed: 'notifications.typeMemberRemoved',
  page_mention: 'notifications.typePageMention',
  page_edited: 'notifications.typePageEdited',
};

export default function NotificationSettings() {
  const { t } = useI18n();
  const { api } = useAuth();
  const { pushEnabled, enablePush, disablePush } = useNotifications();
  const [prefs, setPrefs] = createSignal<NotificationPrefsDTO | null>(null);
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [saving, setSaving] = createSignal(false);

  const loadPrefs = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await api().notifications.preferences.getNotificationPrefs();
      setPrefs(data);
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  createEffect(() => {
    loadPrefs();
  });

  const getChannels = (type: string): ChannelSetDTO => {
    const p = prefs();
    if (!p) return { email: false, web: false };
    const override = p.overrides?.[type];
    if (override) return override;
    return p.defaults?.[type] ?? { email: false, web: false };
  };

  const setChannel = async (type: string, channel: 'email' | 'web', value: boolean) => {
    const p = prefs();
    if (!p) return;
    const current = getChannels(type);
    const updated: ChannelSetDTO = { ...current, [channel]: value };
    const newOverrides = { ...p.overrides, [type]: updated };

    // Optimistic update
    setPrefs({ ...p, overrides: newOverrides });

    try {
      setSaving(true);
      const result = await api().notifications.preferences.updateNotificationPrefs({
        overrides: newOverrides,
      });
      setPrefs(result);
    } catch (err) {
      // Revert
      setPrefs(p);
      setError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setSaving(false);
    }
  };

  const handleTogglePush = async () => {
    if (pushEnabled()) {
      await disablePush();
    } else {
      await enablePush();
    }
  };

  const pushPermission = () => {
    if (typeof Notification === 'undefined') return 'unsupported';
    return Notification.permission;
  };

  return (
    <section class={styles.section}>
      <h3>{t('notifications.preferences')}</h3>
      <Show when={error()}>
        <div class={styles.error}>{error()}</div>
      </Show>

      <div class={styles.pushSection}>
        <button
          class={`${styles.pushButton} ${pushEnabled() ? styles.pushActive : ''}`}
          onClick={handleTogglePush}
          disabled={pushPermission() === 'denied'}
        >
          {pushEnabled() ? t('notifications.pushEnabled') : t('notifications.enablePush')}
        </button>
        <Show when={pushPermission() === 'denied'}>
          <p class={styles.pushDenied}>{t('notifications.pushDenied')}</p>
        </Show>
      </div>

      <Show when={prefs()} fallback={loading() ? <p>{t('common.loading')}</p> : null}>
        <table class={styles.prefsTable}>
          <thead>
            <tr>
              <th />
              <th>{t('notifications.channelEmail')}</th>
              <th>{t('notifications.channelWeb')}</th>
            </tr>
          </thead>
          <tbody>
            <For each={[...NOTIFICATION_TYPES]}>
              {(type) => {
                const channels = () => getChannels(type);
                return (
                  <tr>
                    <td class={styles.typeLabel}>{t(TYPE_LABEL_KEYS[type] ?? type)}</td>
                    <td class={styles.toggleCell}>
                      <input
                        type="checkbox"
                        checked={channels().email}
                        onChange={(e) => setChannel(type, 'email', e.currentTarget.checked)}
                        disabled={saving()}
                      />
                    </td>
                    <td class={styles.toggleCell}>
                      <input
                        type="checkbox"
                        checked={channels().web}
                        onChange={(e) => setChannel(type, 'web', e.currentTarget.checked)}
                        disabled={saving()}
                      />
                    </td>
                  </tr>
                );
              }}
            </For>
          </tbody>
        </table>
      </Show>
    </section>
  );
}
