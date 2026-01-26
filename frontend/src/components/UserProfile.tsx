// User profile page for managing personal settings.

import { createSignal, createEffect, createMemo, Show } from 'solid-js';
import { createApi } from '../useApi';
import type { UserResponse, UserSettings, WorkspaceMembershipSettings } from '../types.gen';
import styles from './UserProfile.module.css';
import { useI18n } from '../i18n';

interface UserProfileProps {
  user: UserResponse;
  token: string;
  onClose: () => void;
}

export default function UserProfile(props: UserProfileProps) {
  const { t } = useI18n();

  // Personal Settings states
  const [theme, setTheme] = createSignal('light');
  const [language, setLanguage] = createSignal('en');
  const [notifications, setNotifications] = createSignal(true);

  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [success, setSuccess] = createSignal<string | null>(null);

  // Create API client
  const api = createMemo(() => createApi(() => props.token));
  const wsApi = createMemo(() => {
    const wsID = props.user.workspace_id;
    return wsID ? api().ws(wsID) : null;
  });

  // Get avatar URL from OAuth identities
  const getAvatarUrl = () => {
    const identities = props.user.oauth_identities;
    if (!identities) return null;
    for (const identity of identities) {
      if (identity.avatar_url) {
        return identity.avatar_url;
      }
    }
    return null;
  };

  // Get initials from user name
  const getInitials = () => {
    const name = props.user.name || props.user.email || '';
    const parts = name.split(/[\s@]+/);
    if (parts.length >= 2 && parts[0] && parts[1]) {
      return ((parts[0][0] || '') + (parts[1][0] || '')).toUpperCase();
    }
    return name.slice(0, 2).toUpperCase();
  };

  createEffect(() => {
    setTheme(props.user.settings?.theme || 'light');
    setLanguage(props.user.settings?.language || 'en');
  });

  const loadData = async () => {
    try {
      setLoading(true);
      setError(null);

      // Load membership settings (notifications)
      const currentWsMembership = props.user.workspaces?.find((m) => m.workspace_id === props.user.workspace_id);
      if (currentWsMembership) {
        setNotifications(currentWsMembership.settings?.notifications ?? true);
      }
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  createEffect(() => {
    loadData();
  });

  const savePersonalSettings = async (e: Event) => {
    e.preventDefault();
    const ws = wsApi();

    try {
      setLoading(true);
      setError(null);
      setSuccess(null);

      const userSettings: UserSettings = {
        theme: theme(),
        language: language(),
      };

      const promises: Promise<unknown>[] = [api().auth.updateUserSettings({ settings: userSettings })];

      // Only update workspace membership settings if we have a workspace
      if (ws) {
        const memSettings: WorkspaceMembershipSettings = {
          notifications: notifications(),
        };
        promises.push(ws.settings.updateWSMembershipSettings({ settings: memSettings }));
      }

      await Promise.all(promises);

      setSuccess(t('success.personalSettingsSaved') || 'Personal settings saved successfully');
    } catch (err) {
      setError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div class={styles.profile}>
      <header class={styles.header}>
        <h2>{t('profile.title')}</h2>
        <button onClick={() => props.onClose()} class={styles.closeButton}>
          &times;
        </button>
      </header>

      <Show when={error()}>
        <div class={styles.error}>{error()}</div>
      </Show>
      <Show when={success()}>
        <div class={styles.success}>{success()}</div>
      </Show>

      <section class={styles.userInfoSection}>
        <div class={styles.avatarLarge}>
          <Show when={getAvatarUrl()} fallback={<span class={styles.initialsLarge}>{getInitials()}</span>}>
            {(url) => (
              <img
                src={url()}
                alt={props.user.name || 'User'}
                class={styles.avatarImageLarge}
                referrerPolicy="no-referrer"
              />
            )}
          </Show>
        </div>
        <div class={styles.userDetails}>
          <h3 class={styles.userName}>{props.user.name}</h3>
          <p class={styles.userEmail}>{props.user.email}</p>
          <Show when={props.user.workspace_role}>
            <span class={styles.userRole}>{props.user.workspace_role}</span>
          </Show>
        </div>
      </section>

      <section class={styles.section}>
        <h3>{t('settings.personalSettings')}</h3>
        <form onSubmit={savePersonalSettings} class={styles.settingsForm}>
          <div class={styles.formItem}>
            <label>{t('settings.theme')}</label>
            <select value={theme()} onChange={(e) => setTheme(e.target.value)}>
              <option value="light">{t('settings.themeLight')}</option>
              <option value="dark">{t('settings.themeDark')}</option>
              <option value="system">{t('settings.themeSystem')}</option>
            </select>
          </div>
          <div class={styles.formItem}>
            <label>{t('settings.language')}</label>
            <select value={language()} onChange={(e) => setLanguage(e.target.value)}>
              <option value="en">{t('settings.languageEn')}</option>
              <option value="fr">{t('settings.languageFr')}</option>
              <option value="de">{t('settings.languageDe')}</option>
              <option value="es">{t('settings.languageEs')}</option>
            </select>
          </div>
          <Show when={props.user.workspace_id}>
            <div class={styles.formItem}>
              <label class={styles.checkboxLabel}>
                <input
                  type="checkbox"
                  checked={notifications()}
                  onChange={(e) => setNotifications(e.currentTarget.checked)}
                />
                {t('settings.enableNotifications')}
              </label>
            </div>
          </Show>
          <button type="submit" class={styles.saveButton} disabled={loading()}>
            {t('settings.saveChanges')}
          </button>
        </form>
      </section>
    </div>
  );
}
