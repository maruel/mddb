// Organization settings panel for managing organization members and preferences.

import { createSignal, createEffect, createMemo, Show } from 'solid-js';
import { useNavigate, useLocation } from '@solidjs/router';
import { useAuth } from '../../contexts';
import { useI18n } from '../../i18n';
import type { UserResponse, OrgInvitationResponse, OrganizationRole, ResourceQuotas } from '@sdk/types.gen';
import { OrgRoleOwner, OrgRoleAdmin } from '@sdk/types.gen';
import MembersTable from './MembersTable';
import InviteForm from './InviteForm';
import ResourceQuotaForm from './ResourceQuotaForm';
import styles from './OrgSettingsPanel.module.css';

interface OrgSettingsPanelProps {
  orgId: string;
  section?: string;
}

type Tab = 'members' | 'settings' | 'quotas';

export default function OrgSettingsPanel(props: OrgSettingsPanelProps) {
  const { t } = useI18n();
  const navigate = useNavigate();
  const location = useLocation();
  const { user, api } = useAuth();

  const getInitialTab = (): Tab => {
    if (props.section === 'settings') return 'settings';
    if (props.section === 'quotas') return 'quotas';
    return 'members';
  };

  const [activeTab, setActiveTab] = createSignal<Tab>(getInitialTab());
  const [members, setMembers] = createSignal<UserResponse[]>([]);
  const [invitations, setInvitations] = createSignal<OrgInvitationResponse[]>([]);

  const [orgName, setOrgName] = createSignal('');
  const [originalOrgName, setOriginalOrgName] = createSignal('');

  // Organization-specific quotas (not part of ResourceQuotas)
  const [maxWorkspacesPerOrg, setMaxWorkspacesPerOrg] = createSignal(0);
  const [maxMembersPerOrg, setMaxMembersPerOrg] = createSignal(0);
  const [maxMembersPerWorkspace, setMaxMembersPerWorkspace] = createSignal(0);
  const [maxTotalStorageBytes, setMaxTotalStorageBytes] = createSignal(0);

  // Resource quotas: -1 = inherit from server, 0 = disabled, positive = limit
  const [resourceQuotas, setResourceQuotas] = createSignal<ResourceQuotas>({
    max_pages: -1,
    max_storage_bytes: -1,
    max_records_per_table: -1,
    max_asset_size_bytes: -1,
    max_tables_per_workspace: -1,
    max_columns_per_table: -1,
  });

  // Server-imposed upper bounds for resource quotas (shown as ceiling hints)
  const [serverLimits, setServerLimits] = createSignal<ResourceQuotas | null>(null);

  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [success, setSuccess] = createSignal<string | null>(null);

  const orgApi = createMemo(() => api().org(props.orgId));

  const userOrgRole = createMemo(() => {
    const membership = user()?.organizations?.find((m) => m.organization_id === props.orgId);
    return membership?.role;
  });

  const isAdmin = createMemo(() => {
    const role = userOrgRole();
    return role === OrgRoleOwner || role === OrgRoleAdmin;
  });

  const handleTabChange = (tab: Tab) => {
    setActiveTab(tab);
    const newHash = tab === 'members' ? '' : `#${tab}`;
    navigate(location.pathname + newHash, { replace: true });
  };

  const loadOrgData = async () => {
    const org = orgApi();
    const orgData = await org.organizations.getOrganization();
    setOrgName(orgData.name);
    setOriginalOrgName(orgData.name);
    setMaxWorkspacesPerOrg(orgData.quotas.max_workspaces_per_org);
    setMaxMembersPerOrg(orgData.quotas.max_members_per_org);
    setMaxMembersPerWorkspace(orgData.quotas.max_members_per_workspace);
    setMaxTotalStorageBytes(orgData.quotas.max_total_storage_bytes);
    setResourceQuotas({
      max_pages: orgData.quotas.max_pages,
      max_storage_bytes: orgData.quotas.max_storage_bytes,
      max_records_per_table: orgData.quotas.max_records_per_table,
      max_asset_size_bytes: orgData.quotas.max_asset_size_bytes,
      max_tables_per_workspace: orgData.quotas.max_tables_per_workspace,
      max_columns_per_table: orgData.quotas.max_columns_per_table,
    });
    setServerLimits(orgData.server_resource_limits);
  };

  const loadData = async () => {
    const org = orgApi();

    try {
      setLoading(true);
      setError(null);

      if (activeTab() === 'members' && isAdmin()) {
        const [membersData, invsData] = await Promise.all([
          org.users.listUsers(),
          org.invitations.listOrgInvitations(),
        ]);
        setMembers(membersData.users?.filter((u): u is UserResponse => !!u) || []);
        setInvitations(invsData.invitations?.filter((i): i is OrgInvitationResponse => !!i) || []);
      }

      if (activeTab() === 'settings' || activeTab() === 'quotas') {
        await loadOrgData();
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
    else if (section === 'members' || !section) setActiveTab('members');
  });

  const handleInvite = async (email: string, role: string) => {
    const org = orgApi();

    try {
      setLoading(true);
      await org.invitations.createOrgInvitation({ email, role: role as 'admin' | 'member' });
      setSuccess(t('success.invitationSent') || 'Invitation sent successfully');
      loadData();
    } catch (err) {
      setError(`${t('errors.failedToInvite')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const handleRemoveMember = async (userId: string) => {
    if (!confirm(t('settings.confirmRemoveMember'))) return;
    const org = orgApi();

    try {
      setLoading(true);
      await org.users.removeOrgMember({ user_id: userId });
      setSuccess(t('success.memberRemoved'));
      loadData();
    } catch (err) {
      setError(`${t('errors.failedToRemoveMember')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const handleUpdateRole = async (userId: string, role: string) => {
    const org = orgApi();

    try {
      setLoading(true);
      await org.users.updateOrgMemberRole({ user_id: userId, role: role as OrganizationRole });
      setSuccess(t('success.roleUpdated') || 'Role updated');
      loadData();
    } catch (err) {
      setError(`${t('errors.failedToUpdateRole')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const buildQuotasPayload = () => ({
    max_workspaces_per_org: maxWorkspacesPerOrg(),
    max_members_per_org: maxMembersPerOrg(),
    max_members_per_workspace: maxMembersPerWorkspace(),
    max_total_storage_bytes: maxTotalStorageBytes(),
    ...resourceQuotas(),
  });

  const saveOrgSettings = async (e: Event) => {
    e.preventDefault();
    const org = orgApi();

    try {
      setLoading(true);
      setError(null);
      setSuccess(null);

      if (orgName() !== originalOrgName() && orgName().trim()) {
        await org.organizations.updateOrganization({ name: orgName().trim() });
        setOriginalOrgName(orgName().trim());
      }

      await org.settings.updateOrgPreferences({ quotas: buildQuotasPayload() });
      setSuccess(t('success.orgSettingsSaved') || 'Organization settings saved');
    } catch (err) {
      setError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const saveOrgQuotas = async (e: Event) => {
    e.preventDefault();
    const org = orgApi();

    try {
      setLoading(true);
      setError(null);
      setSuccess(null);

      await org.settings.updateOrgPreferences({ quotas: buildQuotasPayload() });
      setSuccess(t('success.orgSettingsSaved') || 'Organization settings saved');
    } catch (err) {
      setError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const orgRoleOptions = [
    { value: 'owner', label: t('settings.roleOwner') },
    { value: 'admin', label: t('settings.roleAdmin') },
    { value: 'member', label: t('settings.roleMember') },
  ];

  const inviteRoleOptions = [
    { value: 'admin', label: t('settings.roleAdmin') },
    { value: 'member', label: t('settings.roleMember') },
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
          {t('settings.settings')}
        </button>
        <button class={activeTab() === 'quotas' ? styles.activeTab : ''} onClick={() => handleTabChange('quotas')}>
          {t('settings.quotas')}
        </button>
      </div>

      <Show when={error()}>
        <div class={styles.error}>{error()}</div>
      </Show>
      <Show when={success()}>
        <div class={styles.success}>{success()}</div>
      </Show>

      <Show when={activeTab() === 'members'}>
        <section class={styles.section}>
          <h3>{t('settings.organizationMembers')}</h3>
          <Show when={isAdmin()} fallback={<p>{t('settings.adminOnlyMembers')}</p>}>
            <MembersTable
              members={members()}
              currentUserId={user()?.id || ''}
              roleOptions={orgRoleOptions}
              roleField="org_role"
              onUpdateRole={handleUpdateRole}
              onRemove={handleRemoveMember}
              loading={loading()}
            />
            <InviteForm
              roleOptions={inviteRoleOptions}
              defaultRole="member"
              pendingInvitations={pendingInvitations()}
              onInvite={handleInvite}
              loading={loading()}
            />
          </Show>
        </section>
      </Show>

      <Show when={activeTab() === 'settings'}>
        <section class={styles.section}>
          <h3>{t('settings.organizationPreferences')}</h3>
          <Show when={isAdmin()} fallback={<p>{t('settings.adminOnlySettings')}</p>}>
            <form onSubmit={saveOrgSettings} class={styles.settingsForm}>
              <div class={styles.formItem}>
                <label>{t('settings.organizationName')}</label>
                <input type="text" value={orgName()} onInput={(e) => setOrgName(e.target.value)} required />
              </div>

              <h4>{t('settings.organizationQuotas')}</h4>
              <div class={styles.formGrid}>
                <div class={styles.formItem}>
                  <label>{t('settings.maxWorkspacesPerOrg')}</label>
                  <input
                    type="number"
                    value={maxWorkspacesPerOrg()}
                    onInput={(e) => setMaxWorkspacesPerOrg(parseInt(e.target.value) || 1)}
                    min="1"
                  />
                </div>
                <div class={styles.formItem}>
                  <label>{t('settings.maxMembersPerOrg')}</label>
                  <input
                    type="number"
                    value={maxMembersPerOrg()}
                    onInput={(e) => setMaxMembersPerOrg(parseInt(e.target.value) || 1)}
                    min="1"
                  />
                </div>
                <div class={styles.formItem}>
                  <label>{t('settings.maxMembersPerWorkspace')}</label>
                  <input
                    type="number"
                    value={maxMembersPerWorkspace()}
                    onInput={(e) => setMaxMembersPerWorkspace(parseInt(e.target.value) || 1)}
                    min="1"
                  />
                </div>
                <div class={styles.formItem}>
                  <label>{t('settings.maxTotalStorageBytes')}</label>
                  <input
                    type="number"
                    value={maxTotalStorageBytes()}
                    onInput={(e) => setMaxTotalStorageBytes(parseInt(e.target.value) || 1)}
                    min="1"
                  />
                </div>
              </div>

              <button type="submit" class={styles.saveButton} disabled={loading()}>
                {t('common.save')}
              </button>
            </form>
          </Show>
        </section>
      </Show>

      <Show when={activeTab() === 'quotas'}>
        <section class={styles.section}>
          <h3>{t('settings.quotas')}</h3>
          <Show when={isAdmin()} fallback={<p>{t('settings.adminOnlySettings')}</p>}>
            <form onSubmit={saveOrgQuotas} class={styles.settingsForm}>
              <ResourceQuotaForm
                value={resourceQuotas}
                onChange={setResourceQuotas}
                ceiling={serverLimits}
                ceilingLabel={t('settings.serverCeiling')}
                allowInherit={true}
              />
              <button type="submit" class={styles.saveButton} disabled={loading()}>
                {t('common.save')}
              </button>
            </form>
          </Show>
        </section>
      </Show>
    </div>
  );
}
