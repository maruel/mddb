// Organization settings page for managing organization name, members, and preferences.

import { createSignal, createEffect, createMemo, For, Show } from 'solid-js';
import { createApi } from '../useApi';
import type { UserResponse, OrgInvitationResponse, OrganizationRole } from '../types.gen';
import { OrgRoleOwner, OrgRoleAdmin } from '../types.gen';
import styles from './OrganizationSettings.module.css';
import { useI18n } from '../i18n';

interface OrganizationSettingsProps {
  user: UserResponse;
  token: string;
  orgId: string;
  onBack: () => void;
}

type Tab = 'members' | 'settings';

export default function OrganizationSettings(props: OrganizationSettingsProps) {
  const { t } = useI18n();
  const [activeTab, setActiveTab] = createSignal<Tab>('members');
  const [members, setMembers] = createSignal<UserResponse[]>([]);
  const [invitations, setInvitations] = createSignal<OrgInvitationResponse[]>([]);

  // Form states
  const [inviteEmail, setInviteEmail] = createSignal('');
  const [inviteRole, setInviteRole] = createSignal<'admin' | 'member'>('member');

  // Organization settings states
  const [orgName, setOrgName] = createSignal('');
  const [originalOrgName, setOriginalOrgName] = createSignal('');

  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [success, setSuccess] = createSignal<string | null>(null);

  // Create API client
  const api = createMemo(() => createApi(() => props.token));
  const orgApi = createMemo(() => api().org(props.orgId));

  // Get user's role in this organization
  const userOrgRole = createMemo(() => {
    const membership = props.user.organizations?.find((m) => m.organization_id === props.orgId);
    return membership?.role;
  });

  const isAdmin = createMemo(() => {
    const role = userOrgRole();
    return role === OrgRoleOwner || role === OrgRoleAdmin;
  });

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

  const handleInvite = async (e: Event) => {
    e.preventDefault();
    const org = orgApi();
    if (!inviteEmail()) return;

    try {
      setLoading(true);
      await org.invitations.createOrgInvitation({ email: inviteEmail(), role: inviteRole() });
      setInviteEmail('');
      setSuccess(t('success.invitationSent') || 'Invitation sent successfully');
      loadData();
    } catch (err) {
      setError(`${t('errors.failedToInvite')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const handleUpdateRole = async (userId: string, role: OrganizationRole) => {
    const org = orgApi();

    try {
      setLoading(true);
      await org.users.updateOrgMemberRole({ user_id: userId, role });
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

      // Rename org if name changed
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

  return (
    <div class={styles.settings}>
      <header class={styles.header}>
        <button onClick={() => props.onBack()} class={styles.backButton}>
          ← {t('common.back')}
        </button>
        <h2>{t('settings.organizationSettings')}</h2>
      </header>

      <div class={styles.tabs}>
        <button class={activeTab() === 'members' ? styles.activeTab : ''} onClick={() => setActiveTab('members')}>
          {t('settings.members')}
        </button>
        <button class={activeTab() === 'settings' ? styles.activeTab : ''} onClick={() => setActiveTab('settings')}>
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
                        <Show when={member.id !== props.user.id} fallback={member.org_role}>
                          <select
                            value={member.org_role}
                            onChange={(e) => handleUpdateRole(member.id, e.target.value as OrganizationRole)}
                            class={styles.roleSelect}
                          >
                            <option value="owner">{t('settings.roleOwner')}</option>
                            <option value="admin">{t('settings.roleAdmin')}</option>
                            <option value="member">{t('settings.roleMember')}</option>
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
                  <select value={inviteRole()} onChange={(e) => setInviteRole(e.target.value as 'admin' | 'member')}>
                    <option value="admin">{t('settings.roleAdmin')}</option>
                    <option value="member">{t('settings.roleMember')}</option>
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
