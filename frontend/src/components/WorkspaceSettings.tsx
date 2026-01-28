// Workspace settings page for managing workspace and members.

import { createSignal, createEffect, For, Show } from 'solid-js';
import { useAuth } from '../contexts';
import {
  WSRoleAdmin,
  type UserResponse,
  type WSInvitationResponse,
  type WorkspaceRole,
  type GitRemoteResponse,
} from '@sdk/types.gen';
import styles from './WorkspaceSettings.module.css';
import { useI18n } from '../i18n';

interface WorkspaceSettingsProps {
  onBack: () => void;
  onOpenOrgSettings: () => void;
}

type Tab = 'members' | 'workspace' | 'sync';

export default function WorkspaceSettings(props: WorkspaceSettingsProps) {
  const { t } = useI18n();
  const { user, orgApi, wsApi } = useAuth();

  const [activeTab, setActiveTab] = createSignal<Tab>('members');
  const [members, setMembers] = createSignal<UserResponse[]>([]);
  const [invitations, setInvitations] = createSignal<WSInvitationResponse[]>([]);

  // Git Remote state (single remote per org)
  const [gitRemote, setGitRemote] = createSignal<GitRemoteResponse | null>(null);
  const [newRemoteURL, setNewRemoteURL] = createSignal('');
  const [newRemoteToken, setNewRemoteToken] = createSignal('');

  // Form states
  const [inviteEmail, setInviteEmail] = createSignal('');
  const [inviteRole, setInviteRole] = createSignal<'admin' | 'editor' | 'viewer'>('viewer');

  // Workspace Settings states
  const [wsName, setWsName] = createSignal('');
  const [originalWsName, setOriginalWsName] = createSignal('');

  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [success, setSuccess] = createSignal<string | null>(null);

  const isAdmin = () => user()?.workspace_role === WSRoleAdmin;

  const loadData = async () => {
    const org = orgApi();
    if (!org) return;

    try {
      setLoading(true);
      setError(null);

      const ws = wsApi();
      if (activeTab() === 'members' && isAdmin()) {
        const [membersData, invsData] = await Promise.all([
          org.users.listUsers(),
          ws ? ws.invitations.listWSInvitations() : Promise.resolve({ invitations: [] }),
        ]);
        setMembers(membersData.users?.filter((u): u is UserResponse => !!u) || []);
        setInvitations(invsData.invitations?.filter((i): i is WSInvitationResponse => !!i) || []);
      }

      if (activeTab() === 'workspace' && ws) {
        const wsData = await ws.workspaces.getWorkspace();
        setWsName(wsData.name);
        setOriginalWsName(wsData.name);
      }

      if (activeTab() === 'sync' && isAdmin() && ws) {
        try {
          const remoteData = await ws.settings.git.getGitRemote();
          setGitRemote(remoteData);
        } catch {
          // No remote configured is a valid state
          setGitRemote(null);
        }
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
    const ws = wsApi();
    if (!inviteEmail() || !ws) return;

    try {
      setLoading(true);
      await ws.invitations.createWSInvitation({ email: inviteEmail(), role: inviteRole() });
      setInviteEmail('');
      setSuccess(t('success.invitationSent') || 'Invitation sent successfully');
      loadData();
    } catch (err) {
      setError(`${t('errors.failedToInvite')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const handleUpdateRole = async (userId: string, role: WorkspaceRole) => {
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      await ws.users.updateWSMemberRole({ user_id: userId, role });
      setSuccess(t('success.roleUpdated') || 'Role updated');
      loadData();
    } catch (err) {
      setError(`${t('errors.failedToUpdateRole')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const saveWorkspaceSettings = async (e: Event) => {
    e.preventDefault();
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      setError(null);
      setSuccess(null);

      // Rename workspace if name changed
      if (wsName() !== originalWsName() && wsName().trim()) {
        await ws.workspaces.updateWorkspace({ name: wsName().trim() });
      }

      setOriginalWsName(wsName().trim());
      setSuccess(t('success.workspaceSettingsSaved') || 'Workspace settings saved successfully');
    } catch (err) {
      setError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const handleAddOrUpdateRemote = async (e: Event) => {
    e.preventDefault();
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      setError(null);
      const remoteData = await ws.settings.git.updateGitRemote({
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
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      setError(null);
      setSuccess(null);
      await ws.settings.git.pushGit();
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

    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      setError(null);
      await ws.settings.git.deleteGitRemote();
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
        <button onClick={() => props.onBack()} class={styles.backButton}>
          ← {t('common.back')}
        </button>
        <h2>{t('settings.title')}</h2>
      </header>

      <div class={styles.tabs}>
        <button class={activeTab() === 'members' ? styles.activeTab : ''} onClick={() => setActiveTab('members')}>
          {t('settings.members')}
        </button>
        <button class={activeTab() === 'workspace' ? styles.activeTab : ''} onClick={() => setActiveTab('workspace')}>
          {t('settings.workspace')}
        </button>
        <Show when={isAdmin()}>
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
          <Show when={isAdmin()} fallback={<p>{t('settings.adminOnlyMembers')}</p>}>
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
                        <Show when={member.id !== user()?.id} fallback={member.workspace_role}>
                          <select
                            value={member.workspace_role}
                            onChange={(e) => handleUpdateRole(member.id, e.target.value as WorkspaceRole)}
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
                  <select
                    value={inviteRole()}
                    onChange={(e) => setInviteRole(e.target.value as 'admin' | 'editor' | 'viewer')}
                  >
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

      <Show when={activeTab() === 'workspace'}>
        <section class={styles.section}>
          <h3>{t('settings.workspaceSettings')}</h3>
          <Show when={isAdmin()} fallback={<p>{t('settings.adminOnlyWorkspace')}</p>}>
            <form onSubmit={saveWorkspaceSettings} class={styles.settingsForm}>
              <div class={styles.formItem}>
                <label>{t('settings.workspaceName')}</label>
                <input type="text" value={wsName()} onInput={(e) => setWsName(e.target.value)} required />
              </div>
              <button type="submit" class={styles.saveButton} disabled={loading()}>
                {t('settings.saveWorkspaceSettings')}
              </button>
            </form>
          </Show>

          <div class={styles.orgLink}>
            <p>{t('settings.orgSettingsHint')}</p>
            <button onClick={() => props.onOpenOrgSettings()} class={styles.linkButton}>
              {t('settings.openOrgSettings')} →
            </button>
          </div>
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
