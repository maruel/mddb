// Workspace settings panel for managing workspace members, settings, and git sync.

import { createSignal, createEffect, Show, For } from 'solid-js';
import { useNavigate, useLocation } from '@solidjs/router';
import { useAuth } from '../../contexts';
import { useI18n } from '../../i18n';
import {
  WSRoleAdmin,
  type UserResponse,
  type WSInvitationResponse,
  type WorkspaceRole,
  type GitRemoteResponse,
  type GitHubAppRepoResponse,
  type GitHubAppInstallationResponse,
  type ResourceQuotas,
} from '@sdk/types.gen';
import MembersTable from './MembersTable';
import InviteForm from './InviteForm';
import ResourceQuotaForm from './ResourceQuotaForm';
import styles from './WorkspaceSettingsPanel.module.css';

interface WorkspaceSettingsPanelProps {
  wsId: string;
  section?: string;
  onNavigateToOrgSettings: (orgId: string, orgName: string) => void;
}

type Tab = 'members' | 'settings' | 'quotas' | 'sync';

export default function WorkspaceSettingsPanel(props: WorkspaceSettingsPanelProps) {
  const { t } = useI18n();
  const navigate = useNavigate();
  const location = useLocation();
  const { user, orgApi, wsApi, api } = useAuth();

  const getInitialTab = (): Tab => {
    if (props.section === 'settings') return 'settings';
    if (props.section === 'quotas') return 'quotas';
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

  // GitHub App state
  const [gitHubAppAvailable, setGitHubAppAvailable] = createSignal(false);
  const [ghInstallations, setGhInstallations] = createSignal<GitHubAppInstallationResponse[]>([]);
  const [ghAppRepos, setGhAppRepos] = createSignal<GitHubAppRepoResponse[]>([]);
  const [ghSelectedInstallation, setGhSelectedInstallation] = createSignal('');
  const [ghSelectedRepo, setGhSelectedRepo] = createSignal('');
  const [ghBranch, setGhBranch] = createSignal('main');
  const [ghLoadingRepos, setGhLoadingRepos] = createSignal(false);
  const [syncSetupMode, setSyncSetupMode] = createSignal<'github' | 'manual'>('github');

  // Sync status state
  const [syncStatus, setSyncStatus] = createSignal('');
  const [lastSyncError, setLastSyncError] = createSignal('');

  // Workspace Settings states
  const [wsName, setWsName] = createSignal('');
  const [originalWsName, setOriginalWsName] = createSignal('');
  const [gitAutoPush, setGitAutoPush] = createSignal(false);

  // Resource quotas: -1 = inherit from parent, 0 = disabled, positive = limit
  const [resourceQuotas, setResourceQuotas] = createSignal<ResourceQuotas>({
    max_pages: -1,
    max_storage_bytes: -1,
    max_records_per_table: -1,
    max_asset_size_bytes: -1,
    max_tables_per_workspace: -1,
    max_columns_per_table: -1,
  });

  // Server+org upper bounds for workspace quotas (shown as ceiling hints)
  const [parentLimits, setParentLimits] = createSignal<ResourceQuotas | null>(null);

  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [success, setSuccess] = createSignal<string | null>(null);

  const isAdmin = () => user()?.workspace_role === WSRoleAdmin;

  const handleTabChange = (tab: Tab) => {
    setActiveTab(tab);
    const newHash = tab === 'members' ? '' : `#${tab}`;
    navigate(location.pathname + newHash, { replace: true });
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

      if ((activeTab() === 'settings' || activeTab() === 'quotas') && ws) {
        const wsData = await ws.workspaces.getWorkspace();
        setWsName(wsData.name);
        setOriginalWsName(wsData.name);
        setGitAutoPush(wsData.settings.git_auto_push);
        setResourceQuotas({
          max_pages: wsData.quotas.max_pages,
          max_storage_bytes: wsData.quotas.max_storage_bytes,
          max_records_per_table: wsData.quotas.max_records_per_table,
          max_asset_size_bytes: wsData.quotas.max_asset_size_bytes,
          max_tables_per_workspace: wsData.quotas.max_tables_per_workspace,
          max_columns_per_table: wsData.quotas.max_columns_per_table,
        });
        setParentLimits(wsData.parent_resource_limits);
      }

      if (activeTab() === 'sync' && isAdmin() && ws) {
        try {
          const availResp = await api().githubApp.isGitHubAppAvailable();
          setGitHubAppAvailable(availResp.available);
          if (availResp.available) {
            try {
              const instResp = await api().githubApp.listGitHubAppInstallations();
              setGhInstallations(instResp.installations || []);
            } catch {
              setGhInstallations([]);
            }
          } else {
            setSyncSetupMode('manual');
          }
        } catch {
          setGitHubAppAvailable(false);
          setSyncSetupMode('manual');
        }

        try {
          const remoteData = await ws.settings.git.getGitRemote();
          setGitRemote(remoteData);
        } catch {
          setGitRemote(null);
        }

        try {
          const statusData = await ws.settings.git.getSyncStatus();
          setSyncStatus(statusData.sync_status || '');
          setLastSyncError(statusData.last_sync_error || '');
        } catch {
          // Ignore
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

  createEffect(() => {
    const section = props.section;
    if (section === 'settings') setActiveTab('settings');
    else if (section === 'quotas') setActiveTab('quotas');
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

      if (wsName() !== originalWsName() && wsName().trim()) {
        await ws.workspaces.updateWorkspace({ name: wsName().trim() });
        setOriginalWsName(wsName().trim());
      }

      setSuccess(t('success.workspaceSettingsSaved') || 'Workspace settings saved successfully');
    } catch (err) {
      setError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const saveWorkspaceQuotas = async (e: Event) => {
    e.preventDefault();
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      setError(null);
      setSuccess(null);

      await ws.workspaces.updateWorkspace({ quotas: resourceQuotas() });
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
      await loadData();
    } catch (err) {
      setError(`${t('errors.pushFailed')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const handlePull = async () => {
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      setError(null);
      setSuccess(null);
      await ws.settings.git.pullGit();
      setSuccess(t('success.pullSuccessful') || 'Pull successful');
      await loadData();
    } catch (err) {
      setError(`${t('errors.pullFailed')}: ${err}`);
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

  const handleLoadGhRepos = async (installationId?: string) => {
    const idStr = installationId ?? ghSelectedInstallation();
    const instId = parseInt(idStr, 10);
    if (!instId) return;

    try {
      setGhLoadingRepos(true);
      setGhAppRepos([]);
      setGhSelectedRepo('');
      setError(null);
      const resp = await api().githubApp.listGitHubAppRepos({ installation_id: instId });
      setGhAppRepos(resp.repos || []);
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setGhLoadingRepos(false);
    }
  };

  const handleSetupGitHubApp = async (e: Event) => {
    e.preventDefault();
    const ws = wsApi();
    if (!ws) return;

    const selected = ghAppRepos().find((r) => r.full_name === ghSelectedRepo());
    if (!selected) return;

    try {
      setLoading(true);
      setError(null);
      const remoteData = await ws.settings.git.setupGitHubAppRemote({
        installation_id: parseInt(ghSelectedInstallation(), 10),
        repo_owner: selected.owner,
        repo_name: selected.name,
        branch: ghBranch() || 'main',
      });
      setGitRemote(remoteData);
      setSuccess(t('success.gitHubAppConfigured') || 'GitHub App remote configured');
      setGhAppRepos([]);
      setGhSelectedInstallation('');
      setGhSelectedRepo('');
    } catch (err) {
      setError(`${t('errors.failedToAddRemote')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const handleToggleAutoPush = async () => {
    const ws = wsApi();
    if (!ws) return;

    const newVal = !gitAutoPush();
    try {
      setLoading(true);
      setError(null);
      const wsData = await ws.workspaces.getWorkspace();
      await ws.workspaces.updateWorkspace({
        settings: { ...wsData.settings, git_auto_push: newVal },
      });
      setGitAutoPush(newVal);
    } catch (err) {
      setError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const syncStatusLabel = () => {
    const s = syncStatus();
    switch (s) {
      case 'syncing':
        return t('settings.syncStatusSyncing');
      case 'error':
        return t('settings.syncStatusError');
      case 'conflict':
        return t('settings.syncStatusConflict');
      default:
        return t('settings.syncStatusIdle');
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
        <button class={activeTab() === 'quotas' ? styles.activeTab : ''} onClick={() => handleTabChange('quotas')}>
          {t('settings.quotas')}
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

              <button type="submit" class={styles.saveButton} disabled={loading()}>
                {t('settings.saveWorkspaceSettings')}
              </button>
            </form>

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
          </Show>
        </section>
      </Show>

      <Show when={activeTab() === 'quotas'}>
        <section class={styles.section}>
          <h3>{t('settings.quotas')}</h3>
          <Show when={isAdmin()} fallback={<p>{t('settings.adminOnlyWorkspace')}</p>}>
            <form onSubmit={saveWorkspaceQuotas} class={styles.settingsForm}>
              <ResourceQuotaForm
                value={resourceQuotas}
                onChange={setResourceQuotas}
                ceiling={parentLimits}
                ceilingLabel={t('settings.parentCeiling')}
                allowInherit={true}
              />
              <button type="submit" class={styles.saveButton} disabled={loading()}>
                {t('common.save')}
              </button>
            </form>
          </Show>
        </section>
      </Show>

      <Show when={activeTab() === 'sync'}>
        <section class={styles.section}>
          <h3>{t('settings.gitSynchronization')}</h3>
          <p class={styles.hint}>{t('settings.gitSyncHint')}</p>

          {/* Conflict alert */}
          <Show when={syncStatus() === 'conflict'}>
            <div class={styles.conflictAlert}>
              {t('settings.conflictMessage')}
              <Show when={lastSyncError()}>
                <pre class={styles.conflictFiles}>{lastSyncError()}</pre>
              </Show>
            </div>
          </Show>

          {/* Sync error alert */}
          <Show when={syncStatus() === 'error' && lastSyncError()}>
            <div class={styles.error}>{lastSyncError()}</div>
          </Show>

          <Show when={gitRemote()}>
            {(remote) => (
              <>
                <table class={styles.table}>
                  <thead>
                    <tr>
                      <th>{t('settings.urlColumn')}</th>
                      <Show when={remote().branch}>
                        <th>{t('settings.branchColumn')}</th>
                      </Show>
                      <th>{t('settings.statusColumn')}</th>
                      <th>{t('settings.lastSyncColumn')}</th>
                      <th>{t('common.actions')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      <td>{remote().url}</td>
                      <Show when={remote().branch}>
                        <td>{remote().branch}</td>
                      </Show>
                      <td>
                        <span class={styles.syncBadge} data-status={syncStatus() || 'idle'}>
                          {syncStatusLabel()}
                        </span>
                      </td>
                      <td>
                        {(() => {
                          const ls = remote().last_sync;
                          return ls ? new Date(ls).toLocaleString() : t('settings.never');
                        })()}
                      </td>
                      <td class={styles.actions}>
                        <button onClick={handlePush} disabled={loading()} class={styles.smallButton}>
                          {t('common.push')}
                        </button>
                        <button onClick={handlePull} disabled={loading()} class={styles.smallButton}>
                          {t('settings.pull')}
                        </button>
                        <button onClick={handleDeleteRemote} disabled={loading()} class={styles.deleteButtonSmall}>
                          {t('common.remove')}
                        </button>
                      </td>
                    </tr>
                  </tbody>
                </table>

                {/* Auto-push toggle */}
                <div class={styles.autoPushRow}>
                  <label class={styles.checkboxLabel}>
                    <input type="checkbox" checked={gitAutoPush()} onChange={handleToggleAutoPush} />
                    {t('settings.autoPush')}
                  </label>
                  <p class={styles.hint}>{t('settings.autoPushHint')}</p>
                </div>
              </>
            )}
          </Show>

          <Show when={!gitRemote()}>
            <div class={styles.setupTabs}>
              <button
                class={
                  !gitHubAppAvailable()
                    ? styles.disabledSetupTab
                    : syncSetupMode() === 'github'
                      ? styles.activeSetupTab
                      : styles.setupTab
                }
                onClick={() => gitHubAppAvailable() && setSyncSetupMode('github')}
                disabled={!gitHubAppAvailable()}
              >
                {t('settings.gitHubAppSetup')}
              </button>
              <button
                class={syncSetupMode() === 'manual' ? styles.activeSetupTab : styles.setupTab}
                onClick={() => setSyncSetupMode('manual')}
              >
                {t('settings.manualSetup')}
              </button>
            </div>

            <Show when={syncSetupMode() === 'github' && !gitHubAppAvailable()}>
              <div class={styles.notConfiguredMessage}>{t('settings.gitHubAppNotConfigured')}</div>
            </Show>

            <Show when={syncSetupMode() === 'github' && gitHubAppAvailable()}>
              <div class={styles.addRemoteSection}>
                <h4>{t('settings.gitHubAppSetup')}</h4>
                <form onSubmit={handleSetupGitHubApp} class={styles.settingsForm}>
                  <div class={styles.formItem}>
                    <label>{t('settings.selectInstallation')}</label>
                    <select
                      value={ghSelectedInstallation()}
                      onChange={(e) => {
                        setGhSelectedInstallation(e.target.value);
                        if (e.target.value) handleLoadGhRepos(e.target.value);
                      }}
                      required
                    >
                      <option value="">--</option>
                      <For each={ghInstallations()}>
                        {(inst) => (
                          <option value={String(inst.id)}>
                            {inst.account} ({inst.id})
                          </option>
                        )}
                      </For>
                    </select>
                  </div>

                  <Show when={ghLoadingRepos()}>
                    <p class={styles.hint}>{t('common.loading')}</p>
                  </Show>

                  <Show when={ghAppRepos().length > 0}>
                    <div class={styles.formItem}>
                      <label>{t('settings.selectRepository')}</label>
                      <select value={ghSelectedRepo()} onChange={(e) => setGhSelectedRepo(e.target.value)} required>
                        <option value="">--</option>
                        <For each={ghAppRepos()}>
                          {(repo) => (
                            <option value={repo.full_name}>
                              {repo.full_name}
                              {repo.private ? ' (private)' : ''}
                            </option>
                          )}
                        </For>
                      </select>
                    </div>

                    <div class={styles.formItem}>
                      <label>{t('settings.branchColumn')}</label>
                      <input
                        type="text"
                        value={ghBranch()}
                        onInput={(e) => setGhBranch(e.target.value)}
                        placeholder="main"
                      />
                    </div>

                    <button type="submit" class={styles.saveButton} disabled={loading() || !ghSelectedRepo()}>
                      {t('settings.connectGitHub')}
                    </button>
                  </Show>
                </form>
              </div>
            </Show>

            <Show when={syncSetupMode() === 'manual'}>
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
          </Show>
        </section>
      </Show>
    </div>
  );
}
