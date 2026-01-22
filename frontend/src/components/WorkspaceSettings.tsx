import { createSignal, createEffect, createMemo, For, Show } from 'solid-js';
import { createApi } from '../useApi';
import type {
  UserResponse,
  InvitationResponse,
  UserSettings,
  MembershipSettings,
  OrganizationSettings,
  UserRole,
  GitRemoteResponse,
} from '../types.gen';
import styles from './WorkspaceSettings.module.css';
import { useI18n } from '../i18n';

interface WorkspaceSettingsProps {
  user: UserResponse;
  token: string;
  onClose: () => void;
}

type Tab = 'members' | 'personal' | 'workspace' | 'sync';

export default function WorkspaceSettings(props: WorkspaceSettingsProps) {
  const { t } = useI18n();
  const [activeTab, setActiveTab] = createSignal<Tab>('members');
  const [members, setMembers] = createSignal<UserResponse[]>([]);
  const [invitations, setInvitations] = createSignal<InvitationResponse[]>([]);

  // Git Remote state (single remote per org)
  const [gitRemote, setGitRemote] = createSignal<GitRemoteResponse | null>(null);
  const [newRemoteURL, setNewRemoteURL] = createSignal('');
  const [newRemoteToken, setNewRemoteToken] = createSignal('');

  // Form states
  const [inviteEmail, setInviteEmail] = createSignal('');
  const [inviteRole, setInviteRole] = createSignal<'admin' | 'editor' | 'viewer'>('viewer');

  // Personal Settings states
  const [theme, setTheme] = createSignal('light');
  const [language, setLanguage] = createSignal('en');
  const [notifications, setNotifications] = createSignal(true);

  createEffect(() => {
    setTheme(props.user.settings?.theme || 'light');
    setLanguage(props.user.settings?.language || 'en');
  });

  // Workspace Settings states
  const [orgName, setOrgName] = createSignal('');
  const [publicAccess, setPublicAccess] = createSignal(false);
  const [allowedDomains, setAllowedDomains] = createSignal('');

  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [success, setSuccess] = createSignal<string | null>(null);

  // Create API client
  const api = createMemo(() => createApi(() => props.token));
  const orgApi = createMemo(() => {
    const orgID = props.user.organization_id;
    return orgID ? api().org(orgID) : null;
  });

  const loadData = async () => {
    const org = orgApi();
    if (!org) return;

    try {
      setLoading(true);
      setError(null);

      if (activeTab() === 'members' && props.user.role === 'admin') {
        const [membersData, invsData] = await Promise.all([org.users.list(), org.invitations.list()]);
        setMembers(membersData.users?.filter((u): u is UserResponse => !!u) || []);
        setInvitations(invsData.invitations?.filter((i): i is InvitationResponse => !!i) || []);
      }

      if (activeTab() === 'workspace') {
        const orgData = await org.settings.organization.get();
        setOrgName(orgData.name);
        setPublicAccess(orgData.settings?.public_access || false);
        setAllowedDomains(orgData.settings?.allowed_domains?.join(', ') || '');
      }

      if (activeTab() === 'sync' && props.user.role === 'admin') {
        try {
          const remoteData = await org.settings.git.get();
          setGitRemote(remoteData);
        } catch {
          // No remote configured is a valid state
          setGitRemote(null);
        }
      }

      // Load membership settings (notifications)
      const currentMembership = props.user.memberships?.find((m) => m.organization_id === props.user.organization_id);
      if (currentMembership) {
        setNotifications(currentMembership.settings?.notifications ?? true);
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

  const handleInvite = async (e: Event) => {
    e.preventDefault();
    const org = orgApi();
    if (!inviteEmail() || !org) return;

    try {
      setLoading(true);
      await org.invitations.create({ email: inviteEmail(), role: inviteRole() });
      setInviteEmail('');
      setSuccess(t('success.invitationSent') || 'Invitation sent successfully');
      loadData();
    } catch (err) {
      setError(`${t('errors.failedToInvite')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const handleUpdateRole = async (userId: string, role: UserRole) => {
    const org = orgApi();
    if (!org) return;

    try {
      setLoading(true);
      await org.users.role.update({ user_id: userId, role });
      setSuccess(t('success.roleUpdated') || 'Role updated');
      loadData();
    } catch (err) {
      setError(`${t('errors.failedToUpdateRole')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const savePersonalSettings = async (e: Event) => {
    e.preventDefault();
    const org = orgApi();
    if (!org) return;

    try {
      setLoading(true);
      setError(null);
      setSuccess(null);

      const userSettings: UserSettings = {
        theme: theme(),
        language: language(),
      };

      const memSettings: MembershipSettings = {
        notifications: notifications(),
      };

      await Promise.all([
        api().auth.settings.update({ settings: userSettings }),
        org.settings.membership.update({ settings: memSettings }),
      ]);

      setSuccess(t('success.personalSettingsSaved') || 'Personal settings saved successfully');
    } catch (err) {
      setError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const saveWorkspaceSettings = async (e: Event) => {
    e.preventDefault();
    const org = orgApi();
    if (!org) return;

    try {
      setLoading(true);
      setError(null);
      setSuccess(null);

      const orgSettings: OrganizationSettings = {
        public_access: publicAccess(),
        allowed_domains: allowedDomains()
          ? allowedDomains()
              .split(',')
              .map((d) => d.trim())
          : [],
        git_auto_push: false, // Preserve default
      };

      await org.settings.preferences.update({ settings: orgSettings });
      setSuccess(t('success.workspaceSettingsSaved') || 'Workspace settings saved successfully');
    } catch (err) {
      setError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const handleAddOrUpdateRemote = async (e: Event) => {
    e.preventDefault();
    const org = orgApi();
    if (!org) return;

    try {
      setLoading(true);
      setError(null);
      const remoteData = await org.settings.git.update({
        url: newRemoteURL(),
        token: newRemoteToken(),
        type: 'custom',
        auth_type: newRemoteToken() ? 'token' : 'none',
      });
      setGitRemote(remoteData);
      setSuccess(t('success.remoteAdded') || 'Git remote configured');
      setNewRemoteURL('');
      setNewRemoteToken('');
    } catch (err) {
      setError(`${t('errors.failedToAddRemote')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const handlePush = async () => {
    const org = orgApi();
    if (!org) return;

    try {
      setLoading(true);
      setError(null);
      setSuccess(null);
      await org.settings.git.pushGit();
      setSuccess(t('success.pushSuccessful') || 'Push successful');
      loadData();
    } catch (err) {
      setError(`${t('errors.pushFailed')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteRemote = async () => {
    if (!confirm(t('settings.confirmRemoveRemote') || 'Are you sure you want to remove this remote?')) return;

    const org = orgApi();
    if (!org) return;

    try {
      setLoading(true);
      setError(null);
      await org.settings.git.delete();
      setGitRemote(null);
      setSuccess(t('success.remoteRemoved') || 'Remote removed');
    } catch (err) {
      setError(`${t('errors.failedToRemoveRemote')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div class={styles.settings}>
      <header class={styles.header}>
        <h2>{t('settings.title')}</h2>
        <button onClick={() => props.onClose()} class={styles.closeButton}>
          &times;
        </button>
      </header>

      <div class={styles.tabs}>
        <button class={activeTab() === 'members' ? styles.activeTab : ''} onClick={() => setActiveTab('members')}>
          {t('settings.members')}
        </button>
        <button class={activeTab() === 'personal' ? styles.activeTab : ''} onClick={() => setActiveTab('personal')}>
          {t('settings.personal')}
        </button>
        <button class={activeTab() === 'workspace' ? styles.activeTab : ''} onClick={() => setActiveTab('workspace')}>
          {t('settings.workspace')}
        </button>
        <Show when={props.user.role === 'admin'}>
          <button class={activeTab() === 'sync' ? styles.activeTab : ''} onClick={() => setActiveTab('sync')}>
            {t('settings.gitSync')}
          </button>
        </Show>
      </div>

      <Show when={error()}>
        <div class={styles.error}>{error()}</div>
      </Show>
      <Show when={success()}>
        <div class={styles.success}>{success()}</div>
      </Show>

      <Show when={activeTab() === 'members'}>
        <section class={styles.section}>
          <h3>{t('settings.members')}</h3>
          <Show when={props.user.role === 'admin'} fallback={<p>{t('settings.adminOnlyMembers')}</p>}>
            <table class={styles.table}>
              <thead>
                <tr>
                  <th>{t('settings.nameColumn')}</th>
                  <th>{t('settings.emailColumn')}</th>
                  <th>{t('settings.roleColumn')}</th>
                </tr>
              </thead>
              <tbody>
                <For each={members()}>
                  {(member) => (
                    <tr>
                      <td>{member.name}</td>
                      <td>{member.email}</td>
                      <td>
                        <Show when={member.id !== props.user.id} fallback={member.role}>
                          <select
                            value={member.role}
                            onChange={(e) => handleUpdateRole(member.id, e.target.value as UserRole)}
                            class={styles.roleSelect}
                          >
                            <option value="admin">{t('settings.roleAdmin')}</option>
                            <option value="editor">{t('settings.roleEditor')}</option>
                            <option value="viewer">{t('settings.roleViewer')}</option>
                          </select>
                        </Show>
                      </td>
                    </tr>
                  )}
                </For>
              </tbody>
            </table>

            <div class={styles.inviteSection}>
              <form onSubmit={handleInvite} class={styles.inviteForm}>
                <h4>{t('settings.inviteNewMember')}</h4>
                <div class={styles.formGroup}>
                  <input
                    type="email"
                    placeholder={t('settings.emailPlaceholder') || 'Email address'}
                    value={inviteEmail()}
                    onInput={(e) => setInviteEmail(e.target.value)}
                    required
                  />
                  <select value={inviteRole()} onChange={(e) => setInviteRole(e.target.value as UserRole)}>
                    <option value="admin">{t('settings.roleAdmin')}</option>
                    <option value="editor">{t('settings.roleEditor')}</option>
                    <option value="viewer">{t('settings.roleViewer')}</option>
                  </select>
                  <button type="submit" disabled={loading()}>
                    {t('common.invite')}
                  </button>
                </div>
              </form>

              <Show when={invitations().length > 0}>
                <h4>{t('settings.pendingInvitations')}</h4>
                <table class={styles.table}>
                  <thead>
                    <tr>
                      <th>{t('settings.emailColumn')}</th>
                      <th>{t('settings.roleColumn')}</th>
                      <th>{t('settings.sentColumn')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <For each={invitations()}>
                      {(inv) => (
                        <tr>
                          <td>{inv.email}</td>
                          <td>{inv.role}</td>
                          <td>{new Date(inv.created).toLocaleDateString()}</td>
                        </tr>
                      )}
                    </For>
                  </tbody>
                </table>
              </Show>
            </div>
          </Show>
        </section>
      </Show>

      <Show when={activeTab() === 'personal'}>
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
            <button type="submit" class={styles.saveButton} disabled={loading()}>
              {t('settings.saveChanges')}
            </button>
          </form>
        </section>
      </Show>

      <Show when={activeTab() === 'workspace'}>
        <section class={styles.section}>
          <h3>{t('settings.workspaceSettings')}</h3>
          <Show when={props.user.role === 'admin'} fallback={<p>{t('settings.adminOnlyWorkspace')}</p>}>
            <form onSubmit={saveWorkspaceSettings} class={styles.settingsForm}>
              <div class={styles.formItem}>
                <label>{t('settings.organizationName')}</label>
                <input
                  type="text"
                  value={orgName()}
                  onInput={(e) => setOrgName(e.target.value)}
                  disabled
                  title={t('settings.renameNotSupported') || 'Rename is not supported yet'}
                />
              </div>
              <div class={styles.formItem}>
                <label class={styles.checkboxLabel}>
                  <input
                    type="checkbox"
                    checked={publicAccess()}
                    onChange={(e) => setPublicAccess(e.currentTarget.checked)}
                  />
                  {t('settings.allowPublicAccess')}
                </label>
              </div>
              <div class={styles.formItem}>
                <label>{t('settings.allowedDomains')}</label>
                <input
                  type="text"
                  placeholder={t('settings.allowedDomainsPlaceholder') || 'example.com, company.org'}
                  value={allowedDomains()}
                  onInput={(e) => setAllowedDomains(e.target.value)}
                />
                <p class={styles.hint}>{t('settings.allowedDomainsHint')}</p>
              </div>
              <button type="submit" class={styles.saveButton} disabled={loading()}>
                {t('settings.saveWorkspaceSettings')}
              </button>
            </form>
          </Show>
        </section>
      </Show>

      <Show when={activeTab() === 'sync'}>
        <section class={styles.section}>
          <h3>{t('settings.gitSynchronization')}</h3>
          <p class={styles.hint}>{t('settings.gitSyncHint')}</p>

          <Show when={gitRemote()}>
            {(remote) => {
              const lastSync = remote().last_sync;
              return (
                <table class={styles.table}>
                  <thead>
                    <tr>
                      <th>{t('settings.urlColumn')}</th>
                      <th>{t('settings.lastSyncColumn')}</th>
                      <th>{t('common.actions')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      <td>{remote().url}</td>
                      <td>{lastSync ? new Date(lastSync).toLocaleString() : t('settings.never')}</td>
                      <td class={styles.actions}>
                        <button onClick={handlePush} disabled={loading()} class={styles.smallButton}>
                          {t('common.push')}
                        </button>
                        <button onClick={handleDeleteRemote} disabled={loading()} class={styles.deleteButtonSmall}>
                          {t('common.remove')}
                        </button>
                      </td>
                    </tr>
                  </tbody>
                </table>
              );
            }}
          </Show>

          <Show when={!gitRemote()}>
            <div class={styles.addRemoteSection}>
              <h4>{t('settings.addNewRemote')}</h4>
              <form onSubmit={handleAddOrUpdateRemote} class={styles.settingsForm}>
                <div class={styles.formItem}>
                  <label>{t('settings.repositoryUrl')}</label>
                  <input
                    type="url"
                    value={newRemoteURL()}
                    onInput={(e) => setNewRemoteURL(e.target.value)}
                    placeholder={t('settings.repositoryUrlPlaceholder') || 'https://github.com/user/repo.git'}
                    required
                  />
                </div>
                <div class={styles.formItem}>
                  <label>{t('settings.personalAccessToken')}</label>
                  <input
                    type="password"
                    value={newRemoteToken()}
                    onInput={(e) => setNewRemoteToken(e.target.value)}
                    placeholder={t('settings.tokenPlaceholder') || 'ghp_...'}
                  />
                  <p class={styles.hint}>{t('settings.tokenHint')}</p>
                </div>
                <button type="submit" class={styles.saveButton} disabled={loading()}>
                  {t('settings.addRemote')}
                </button>
              </form>
            </div>
          </Show>
        </section>
      </Show>
    </div>
  );
}
