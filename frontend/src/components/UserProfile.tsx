// User profile page for managing personal settings.

import { createSignal, createEffect, Show, For } from 'solid-js';
import type { UserSettings, WorkspaceMembershipSettings, OrgMembershipResponse } from '@sdk/types.gen';
import { OrgRoleAdmin, OrgRoleOwner } from '@sdk/types.gen';
import { useAuth } from '../contexts';
import { slugify } from '../utils/urls';
import styles from './UserProfile.module.css';
import { useI18n, type Locale } from '../i18n';

interface UserProfileProps {
  onBack: () => void;
  onOrgSettings: (org: OrgMembershipResponse) => void;
}

export default function UserProfile(props: UserProfileProps) {
  const { t, setLocale } = useI18n();
  const { user, api, wsApi } = useAuth();

  // Personal Settings states
  const [theme, setTheme] = createSignal('light');
  const [language, setLanguage] = createSignal('en');
  const [notifications, setNotifications] = createSignal(true);

  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [success, setSuccess] = createSignal<string | null>(null);

  // Get avatar URL from OAuth identities
  const getAvatarUrl = () => {
    const identities = user()?.oauth_identities;
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
    const u = user();
    const name = u?.name || u?.email || '';
    const parts = name.split(/[\s@]+/);
    if (parts.length >= 2 && parts[0] && parts[1]) {
      return ((parts[0][0] || '') + (parts[1][0] || '')).toUpperCase();
    }
    return name.slice(0, 2).toUpperCase();
  };

  createEffect(() => {
    const u = user();
    setTheme(u?.settings?.theme || 'light');
    setLanguage(u?.settings?.language || 'en');
  });

  const loadData = async () => {
    const u = user();
    if (!u) return;

    try {
      setLoading(true);
      setError(null);

      // Load membership settings (notifications)
      const currentWsMembership = u.workspaces?.find((m) => m.workspace_id === u.workspace_id);
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

      // Update locale immediately so UI refreshes
      const lang = language() as Locale;
      setLocale(lang);
      localStorage.setItem('mddb_locale', lang);

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
        <button onClick={() => props.onBack()} class={styles.backButton}>
          ← {t('common.back')}
        </button>
        <h2>{t('profile.title')}</h2>
      </header>

      <Show when={error()}>
        <div class={styles.error}>{error()}</div>
      </Show>
      <Show when={success()}>
        <div class={styles.success}>{success()}</div>
      </Show>

      <Show when={user()}>
        {(u) => (
          <>
            <section class={styles.userInfoSection}>
              <div class={styles.avatarLarge}>
                <Show when={getAvatarUrl()} fallback={<span class={styles.initialsLarge}>{getInitials()}</span>}>
                  {(url) => (
                    <img
                      src={url()}
                      alt={u().name || 'User'}
                      class={styles.avatarImageLarge}
                      referrerPolicy="no-referrer"
                    />
                  )}
                </Show>
              </div>
              <div class={styles.userDetails}>
                <h3 class={styles.userName}>{u().name}</h3>
                <p class={styles.userEmail}>{u().email}</p>
                <Show when={u().workspace_role}>
                  <span class={styles.userRole}>{u().workspace_role}</span>
                </Show>
              </div>
            </section>

            <section class={styles.section}>
              <h3>{t('profile.myOrganizations')}</h3>
              <Show
                when={u().organizations?.length}
                fallback={<p class={styles.emptyState}>{t('profile.noOrganizations')}</p>}
              >
                <div class={styles.orgList}>
                  <For each={u().organizations}>
                    {(org) => {
                      const isAdmin = () => org.role === OrgRoleAdmin || org.role === OrgRoleOwner;
                      const orgSlug = slugify(org.organization_name || 'organization');
                      const settingsHref = `/o/${org.organization_id}+${orgSlug}/settings`;
                      const orgDisplayName = org.organization_name || org.organization_id;
                      return (
                        <div class={styles.orgItem}>
                          <div class={styles.orgInfo}>
                            <Show when={isAdmin()} fallback={<span class={styles.orgName}>{orgDisplayName}</span>}>
                              <a
                                href={settingsHref}
                                class={styles.orgNameLink}
                                onClick={(e) => {
                                  e.preventDefault();
                                  props.onOrgSettings(org);
                                }}
                              >
                                {orgDisplayName}
                              </a>
                            </Show>
                            <span class={styles.orgRole}>{org.role}</span>
                          </div>
                        </div>
                      );
                    }}
                  </For>
                </div>
              </Show>
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
                <Show when={u().workspace_id}>
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
          </>
        )}
      </Show>
    </div>
  );
}
