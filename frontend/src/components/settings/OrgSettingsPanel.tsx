// Organization settings panel for managing organization members and preferences.

import { createSignal, createEffect, createMemo, Show } from 'solid-js';
import { useAuth } from '../../contexts';
import { useI18n } from '../../i18n';
import type { UserResponse, OrgInvitationResponse, OrganizationRole } from '@sdk/types.gen';
import { OrgRoleOwner, OrgRoleAdmin } from '@sdk/types.gen';
import MembersTable from './MembersTable';
import InviteForm from './InviteForm';
import styles from './OrgSettingsPanel.module.css';

interface OrgSettingsPanelProps {
  orgId: string;
  section?: string;
}

type Tab = 'members' | 'settings';

export default function OrgSettingsPanel(props: OrgSettingsPanelProps) {
  const { t } = useI18n();
  const { user, api } = useAuth();

  // Determine initial tab from section prop
  const getInitialTab = (): Tab => {
    if (props.section === 'settings') return 'settings';
    return 'members';
  };

  const [activeTab, setActiveTab] = createSignal<Tab>(getInitialTab());
  const [members, setMembers] = createSignal<UserResponse[]>([]);
  const [invitations, setInvitations] = createSignal<OrgInvitationResponse[]>([]);

  // Organization settings states
  const [orgName, setOrgName] = createSignal('');
  const [originalOrgName, setOriginalOrgName] = createSignal('');

  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [success, setSuccess] = createSignal<string | null>(null);

  // Create org-scoped API client for the specified orgId
  const orgApi = createMemo(() => api().org(props.orgId));

  // Get user's role in this organization
  const userOrgRole = createMemo(() => {
    const membership = user()?.organizations?.find((m) => m.organization_id === props.orgId);
    return membership?.role;
  });

  const isAdmin = createMemo(() => {
    const role = userOrgRole();
    return role === OrgRoleOwner || role === OrgRoleAdmin;
  });

  // Update URL hash when tab changes
  const handleTabChange = (tab: Tab) => {
    setActiveTab(tab);
    const newHash = tab === 'members' ? '' : `#${tab}`;
    const basePath = window.location.pathname;
    window.history.replaceState(null, '', basePath + newHash);
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

      if (activeTab() === 'settings') {
        const orgData = await org.organizations.getOrganization();
        setOrgName(orgData.name);
        setOriginalOrgName(orgData.name);
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

  const saveOrgSettings = async (e: Event) => {
    e.preventDefault();
    const org = orgApi();

    try {
      setLoading(true);
      setError(null);
      setSuccess(null);

      if (orgName() !== originalOrgName() && orgName().trim()) {
        await org.organizations.updateOrganization({ name: orgName().trim() });
      }

      setOriginalOrgName(orgName().trim());
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
