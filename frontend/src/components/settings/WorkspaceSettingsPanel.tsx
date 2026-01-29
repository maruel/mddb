// Workspace settings panel for managing workspace members, settings, and git sync.

import { createSignal, createEffect, Show } from 'solid-js';
import { useAuth } from '../../contexts';
import { useI18n } from '../../i18n';
import {
  WSRoleAdmin,
  type UserResponse,
  type WSInvitationResponse,
  type WorkspaceRole,
  type GitRemoteResponse,
} from '@sdk/types.gen';
import MembersTable from './MembersTable';
import InviteForm from './InviteForm';
import styles from './WorkspaceSettingsPanel.module.css';

interface WorkspaceSettingsPanelProps {
  wsId: string;
  section?: string;
  onNavigateToOrgSettings: (orgId: string, orgName: string) => void;
}

type Tab = 'members' | 'settings' | 'sync';

export default function WorkspaceSettingsPanel(props: WorkspaceSettingsPanelProps) {
  const { t } = useI18n();
  const { user, orgApi, wsApi } = useAuth();

  // Determine initial tab from section prop
  const getInitialTab = (): Tab => {
    if (props.section === 'settings') return 'settings';
    if (props.section === 'sync') return 'sync';
    return 'members';
  };

  const [activeTab, setActiveTab] = createSignal<Tab>(getInitialTab());
  const [members, setMembers] = createSignal<UserResponse[]>([]);
  const [invitations, setInvitations] = createSignal<WSInvitationResponse[]>([]);

  // Git Remote state
  const [gitRemote, setGitRemote] = createSignal<GitRemoteResponse | null>(null);
  const [newRemoteURL, setNewRemoteURL] = createSignal('');
  const [newRemoteToken, setNewRemoteToken] = createSignal('');

  // Workspace Settings states
  const [wsName, setWsName] = createSignal('');
  const [originalWsName, setOriginalWsName] = createSignal('');

  // Workspace Quotas
  const [maxPages, setMaxPages] = createSignal(0);
  const [maxStorageBytes, setMaxStorageBytes] = createSignal(0);
  const [maxRecordsPerTable, setMaxRecordsPerTable] = createSignal(0);
  const [maxAssetSizeBytes, setMaxAssetSizeBytes] = createSignal(0);

  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [success, setSuccess] = createSignal<string | null>(null);

  const isAdmin = () => user()?.workspace_role === WSRoleAdmin;

  // Update URL hash when tab changes
  const handleTabChange = (tab: Tab) => {
    setActiveTab(tab);
    const newHash = tab === 'members' ? '' : `#${tab}`;
    const basePath = window.location.pathname;
    window.history.replaceState(null, '', basePath + newHash);
  };

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

      if (activeTab() === 'settings' && ws) {
        const wsData = await ws.workspaces.getWorkspace();
        setWsName(wsData.name);
        setOriginalWsName(wsData.name);
        // Load quotas
        setMaxPages(wsData.quotas.max_pages);
        setMaxStorageBytes(wsData.quotas.max_storage_bytes);
        setMaxRecordsPerTable(wsData.quotas.max_records_per_table);
        setMaxAssetSizeBytes(wsData.quotas.max_asset_size_bytes);
      }

      if (activeTab() === 'sync' && isAdmin() && ws) {
        try {
          const remoteData = await ws.settings.git.getGitRemote();
          setGitRemote(remoteData);
        } catch {
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

  // Update tab when section prop changes
  createEffect(() => {
    const section = props.section;
    if (section === 'settings') setActiveTab('settings');
    else if (section === 'sync') setActiveTab('sync');
    else if (section === 'members' || !section) setActiveTab('members');
  });

  const handleInvite = async (email: string, role: string) => {
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      await ws.invitations.createWSInvitation({ email, role: role as 'admin' | 'editor' | 'viewer' });
      setSuccess(t('success.invitationSent') || 'Invitation sent successfully');
      loadData();
    } catch (err) {
      setError(`${t('errors.failedToInvite')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const handleUpdateRole = async (userId: string, role: string) => {
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      await ws.users.updateWSMemberRole({ user_id: userId, role: role as WorkspaceRole });
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

      // Update name if changed
      if (wsName() !== originalWsName() && wsName().trim()) {
        await ws.workspaces.updateWorkspace({ name: wsName().trim() });
        setOriginalWsName(wsName().trim());
      }

      // Update quotas
      await ws.workspaces.updateWorkspace({
        quotas: {
          max_pages: maxPages(),
          max_storage_bytes: maxStorageBytes(),
          max_records_per_table: maxRecordsPerTable(),
          max_asset_size_bytes: maxAssetSizeBytes(),
        },
      });

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

  const wsRoleOptions = [
    { value: 'admin', label: t('settings.roleAdmin') },
    { value: 'editor', label: t('settings.roleEditor') },
    { value: 'viewer', label: t('settings.roleViewer') },
  ];

  const pendingInvitations = () =>
    invitations().map((inv) => ({
      email: inv.email,
      role: inv.role,
      created: inv.created,
    }));

  return (
    <div class={styles.panel}>
      <div class={styles.tabs}>
        <button class={activeTab() === 'members' ? styles.activeTab : ''} onClick={() => handleTabChange('members')}>
          {t('settings.members')}
        </button>
        <button class={activeTab() === 'settings' ? styles.activeTab : ''} onClick={() => handleTabChange('settings')}>
          {t('settings.workspace')}
        </button>
        <Show when={isAdmin()}>
          <button class={activeTab() === 'sync' ? styles.activeTab : ''} onClick={() => handleTabChange('sync')}>
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
            <MembersTable
              members={members()}
              currentUserId={user()?.id || ''}
              roleOptions={wsRoleOptions}
              roleField="workspace_role"
              onUpdateRole={handleUpdateRole}
              loading={loading()}
            />
            <InviteForm
              roleOptions={wsRoleOptions}
              defaultRole="viewer"
              pendingInvitations={pendingInvitations()}
              onInvite={handleInvite}
              loading={loading()}
            />
          </Show>
        </section>
      </Show>

      <Show when={activeTab() === 'settings'}>
        <section class={styles.section}>
          <h3>{t('settings.workspaceSettings')}</h3>
          <Show when={isAdmin()} fallback={<p>{t('settings.adminOnlyWorkspace')}</p>}>
            <form onSubmit={saveWorkspaceSettings} class={styles.settingsForm}>
              <div class={styles.formItem}>
                <label>{t('settings.workspaceName')}</label>
                <input type="text" value={wsName()} onInput={(e) => setWsName(e.target.value)} required />
              </div>

              <h4>{t('settings.workspaceQuotas')}</h4>
              <div class={styles.formGrid}>
                <div class={styles.formItem}>
                  <label>{t('settings.maxPages')}</label>
                  <input
                    type="number"
                    value={maxPages()}
                    onInput={(e) => setMaxPages(parseInt(e.target.value) || 1)}
                    min="1"
                  />
                </div>
                <div class={styles.formItem}>
                  <label>{t('settings.maxStorageBytes')}</label>
                  <input
                    type="number"
                    value={maxStorageBytes()}
                    onInput={(e) => setMaxStorageBytes(parseInt(e.target.value) || 1)}
                    min="1"
                  />
                </div>
                <div class={styles.formItem}>
                  <label>{t('settings.maxRecordsPerTable')}</label>
                  <input
                    type="number"
                    value={maxRecordsPerTable()}
                    onInput={(e) => setMaxRecordsPerTable(parseInt(e.target.value) || 1)}
                    min="1"
                  />
                </div>
                <div class={styles.formItem}>
                  <label>{t('settings.maxAssetSizeBytes')}</label>
                  <input
                    type="number"
                    value={maxAssetSizeBytes()}
                    onInput={(e) => setMaxAssetSizeBytes(parseInt(e.target.value) || 1)}
                    min="1"
                  />
                </div>
              </div>

              <button type="submit" class={styles.saveButton} disabled={loading()}>
                {t('settings.saveWorkspaceSettings')}
              </button>
            </form>
          </Show>

          <div class={styles.orgLink}>
            <p>{t('settings.orgSettingsHint')}</p>
            <button
              onClick={() => {
                const u = user();
                const orgId = u?.organization_id;
                const orgMembership = u?.organizations?.find((m) => m.organization_id === orgId);
                const orgName = orgMembership?.organization_name || '';
                if (orgId) {
                  props.onNavigateToOrgSettings(orgId, orgName);
                }
              }}
              class={styles.linkButton}
            >
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
