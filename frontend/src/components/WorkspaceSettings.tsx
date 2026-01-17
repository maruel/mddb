import { createSignal, createEffect, For, Show } from 'solid-js';
import type {
  User,
  Invitation,
  ListUsersResponse,
  ListInvitationsResponse,
  Organization,
  UserSettings,
  MembershipSettings,
  OrganizationSettings,
  UserRole,
} from '../types';
import styles from './WorkspaceSettings.module.css';

interface WorkspaceSettingsProps {
  user: User;
  token: string;
  onClose: () => void;
}

type Tab = 'members' | 'personal' | 'workspace';

export default function WorkspaceSettings(props: WorkspaceSettingsProps) {
  const [activeTab, setActiveTab] = createSignal<Tab>('members');
  const [members, setMembers] = createSignal<User[]>([]);
  const [invitations, setInvitations] = createSignal<Invitation[]>([]);

  // Form states
  const [inviteEmail, setInviteEmail] = createSignal('');
  const [inviteRole, setInviteRole] = createSignal<'admin' | 'editor' | 'viewer'>('viewer');

  // Personal Settings states
  const [theme, setTheme] = createSignal('light');
  const [language, setLanguage] = createSignal('en');
  const [notifications, setNotifications] = createSignal(true); // Default

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

  const authFetch = async (url: string, options: RequestInit = {}) => {
    let finalUrl = url;
    if (url.startsWith('/api/') && !url.startsWith('/api/auth/')) {
      finalUrl = `/api/${props.user.organization_id}${url.substring(4)}`;
    }

    const res = await fetch(finalUrl, {
      ...options,
      headers: {
        ...options.headers,
        Authorization: `Bearer ${props.token}`,
      },
    });
    if (!res.ok) {
      const data = await res.json();
      throw new Error(data.error?.message || data.error || 'Request failed');
    }
    return res;
  };

  const loadData = async () => {
    try {
      setLoading(true);
      setError(null);

      const promises: Promise<Response>[] = [];

      if (activeTab() === 'members' && props.user.role === 'admin') {
        promises.push(authFetch('/api/users'));
        promises.push(authFetch('/api/invitations'));
      }

      if (activeTab() === 'workspace') {
        promises.push(authFetch('/api/settings/organization'));
      }

      if (promises.length === 0) return;

      const results = await Promise.all(promises);

      if (activeTab() === 'members' && props.user.role === 'admin') {
        const membersData = (await results[0].json()) as ListUsersResponse;
        const invsData = (await results[1].json()) as ListInvitationsResponse;
        setMembers((membersData.users?.filter(Boolean) as User[]) || []);
        setInvitations((invsData.invitations?.filter(Boolean) as Invitation[]) || []);
      }

      if (activeTab() === 'workspace') {
        const orgData = (await results[results.length - 1].json()) as Organization;
        setOrgName(orgData.name);
        setPublicAccess(orgData.settings?.public_access || false);
        setAllowedDomains(orgData.settings?.allowed_domains?.join(', ') || '');
      }

      // Load membership settings (notifications)
      const currentMembership = props.user.memberships?.find(
        (m) => m.organization_id === props.user.organization_id
      );
      if (currentMembership) {
        setNotifications(currentMembership.settings?.notifications ?? true);
      }
    } catch (err) {
      setError('Failed to load settings: ' + err);
    } finally {
      setLoading(false);
    }
  };

  createEffect(() => {
    loadData();
  });

  const handleInvite = async (e: Event) => {
    e.preventDefault();
    if (!inviteEmail()) return;

    try {
      setLoading(true);
      await authFetch('/api/invitations', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: inviteEmail(), role: inviteRole() }),
      });
      setInviteEmail('');
      setSuccess('Invitation sent successfully');
      loadData();
    } catch (err) {
      setError('Failed to send invitation: ' + err);
    } finally {
      setLoading(false);
    }
  };

  const handleUpdateRole = async (userId: string, role: string) => {
    try {
      setLoading(true);
      await authFetch('/api/users/role', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ user_id: userId, role }),
      });
      setSuccess('Role updated');
      loadData();
    } catch (err) {
      setError('Failed to update role: ' + err);
    } finally {
      setLoading(false);
    }
  };

  const savePersonalSettings = async (e: Event) => {
    e.preventDefault();
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
        authFetch('/api/auth/settings', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ settings: userSettings }),
        }),
        authFetch('/api/settings/membership', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ settings: memSettings }),
        }),
      ]);

      setSuccess('Personal settings saved successfully');
    } catch (err) {
      setError('Failed to save settings: ' + err);
    } finally {
      setLoading(false);
    }
  };

  const saveWorkspaceSettings = async (e: Event) => {
    e.preventDefault();
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
      };

      await authFetch('/api/settings/organization', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ settings: orgSettings }),
      });

      setSuccess('Workspace settings saved successfully');
    } catch (err) {
      setError('Failed to save workspace settings: ' + err);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div class={styles.settings}>
      <header class={styles.header}>
        <h2>Settings</h2>
        <button onClick={() => props.onClose()} class={styles.closeButton}>
          &times;
        </button>
      </header>

      <div class={styles.tabs}>
        <button
          class={activeTab() === 'members' ? styles.activeTab : ''}
          onClick={() => setActiveTab('members')}
        >
          Members
        </button>
        <button
          class={activeTab() === 'personal' ? styles.activeTab : ''}
          onClick={() => setActiveTab('personal')}
        >
          Personal
        </button>
        <button
          class={activeTab() === 'workspace' ? styles.activeTab : ''}
          onClick={() => setActiveTab('workspace')}
        >
          Workspace
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
          <h3>Members</h3>
          <Show when={props.user.role === 'admin'} fallback={<p>Only admins can view member list.</p>}>
            <table class={styles.table}>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Email</th>
                  <th>Role</th>
                </tr>
              </thead>
              <tbody>
                <For each={members()}>
                  {(member) => (
                    <tr>
                      <td>{member.name}</td>
                      <td>{member.email}</td>
                      <td>
                        <Show
                          when={member.id !== props.user.id}
                          fallback={member.role}
                        >
                          <select
                            value={member.role}
                            onChange={(e) => handleUpdateRole(member.id, e.target.value)}
                            class={styles.roleSelect}
                          >
                            <option value="admin">Admin</option>
                            <option value="editor">Editor</option>
                            <option value="viewer">Viewer</option>
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
                <h4>Invite new member</h4>
                <div class={styles.formGroup}>
                  <input
                    type="email"
                    placeholder="Email address"
                    value={inviteEmail()}
                    onInput={(e) => setInviteEmail(e.target.value)}
                    required
                  />
                  <select
                    value={inviteRole()}
                    onChange={(e) => setInviteRole(e.target.value as UserRole)}
                  >
                    <option value="admin">Admin</option>
                    <option value="editor">Editor</option>
                    <option value="viewer">Viewer</option>
                  </select>
                  <button type="submit" disabled={loading()}>
                    Invite
                  </button>
                </div>
              </form>

              <Show when={invitations().length > 0}>
                <h4>Pending Invitations</h4>
                <table class={styles.table}>
                  <thead>
                    <tr>
                      <th>Email</th>
                      <th>Role</th>
                      <th>Sent</th>
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
          <h3>Personal Settings</h3>
          <form onSubmit={savePersonalSettings} class={styles.settingsForm}>
            <div class={styles.formItem}>
              <label>Theme</label>
              <select value={theme()} onChange={(e) => setTheme(e.target.value)}>
                <option value="light">Light</option>
                <option value="dark">Dark</option>
                <option value="system">System</option>
              </select>
            </div>
            <div class={styles.formItem}>
              <label>Language</label>
              <select value={language()} onChange={(e) => setLanguage(e.target.value)}>
                <option value="en">English</option>
                <option value="fr">French</option>
                <option value="de">German</option>
                <option value="es">Spanish</option>
              </select>
            </div>
            <div class={styles.formItem}>
              <label class={styles.checkboxLabel}>
                <input
                  type="checkbox"
                  checked={notifications()}
                  onChange={(e) => setNotifications(e.currentTarget.checked)}
                />
                Enable notifications for this workspace
              </label>
            </div>
            <button type="submit" class={styles.saveButton} disabled={loading()}>
              Save Changes
            </button>
          </form>
        </section>
      </Show>

      <Show when={activeTab() === 'workspace'}>
        <section class={styles.section}>
          <h3>Workspace Settings</h3>
          <Show when={props.user.role === 'admin'} fallback={<p>Only admins can modify workspace settings.</p>}>
            <form onSubmit={saveWorkspaceSettings} class={styles.settingsForm}>
              <div class={styles.formItem}>
                <label>Organization Name</label>
                <input
                  type="text"
                  value={orgName()}
                  onInput={(e) => setOrgName(e.target.value)}
                  disabled
                  title="Rename is not supported yet"
                />
              </div>
              <div class={styles.formItem}>
                <label class={styles.checkboxLabel}>
                  <input
                    type="checkbox"
                    checked={publicAccess()}
                    onChange={(e) => setPublicAccess(e.currentTarget.checked)}
                  />
                  Allow public access (Read-only)
                </label>
              </div>
              <div class={styles.formItem}>
                <label>Allowed Email Domains (comma separated)</label>
                <input
                  type="text"
                  placeholder="example.com, company.org"
                  value={allowedDomains()}
                  onInput={(e) => setAllowedDomains(e.target.value)}
                />
                <p class={styles.hint}>Users with these email domains can join automatically.</p>
              </div>
              <button type="submit" class={styles.saveButton} disabled={loading()}>
                Save Workspace Settings
              </button>
            </form>
          </Show>
        </section>
      </Show>
    </div>
  );
}
