import { createSignal, createEffect, For, Show } from 'solid-js';
import type { User, Invitation, ListUsersResponse, ListInvitationsResponse } from '../types';
import styles from './WorkspaceSettings.module.css';

interface WorkspaceSettingsProps {
  user: User;
  token: string;
  onClose: () => void;
}

export default function WorkspaceSettings(props: WorkspaceSettingsProps) {
  const [members, setMembers] = createSignal<User[]>([]);
  const [invitations, setInvitations] = createSignal<Invitation[]>([]);
  const [inviteEmail, setInviteEmail] = createSignal('');
  const [inviteRole, setInviteRole] = createSignal<'admin' | 'editor' | 'viewer'>('viewer');
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);

  const authFetch = async (url: string, options: RequestInit = {}) => {
    let finalUrl = url;
    if (url.startsWith('/api/') && !url.startsWith('/api/auth/')) {
      finalUrl = `/api/${props.user.organization_id}${url.substring(4)}`;
    }

    const res = await fetch(finalUrl, {
      ...options,
      headers: {
        ...options.headers,
        'Authorization': `Bearer ${props.token}`,
      },
    });
    if (!res.ok) {
      const data = await res.json();
      throw new Error(data.error || 'Request failed');
    }
    return res;
  };

  const loadData = async () => {
    try {
      setLoading(true);
      const [membersRes, invsRes] = await Promise.all([
        authFetch('/api/users'),
        authFetch('/api/invitations')
      ]);
      
      const membersData = (await membersRes.json()) as ListUsersResponse;
      const invsData = (await invsRes.json()) as ListInvitationsResponse;
      
      setMembers((membersData.users?.filter(Boolean) as User[]) || []);
      setInvitations((invsData.invitations?.filter(Boolean) as Invitation[]) || []);
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
      loadData();
    } catch (err) {
      setError('Failed to update role: ' + err);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div class={styles.settings}>
      <header class={styles.header}>
        <h2>Workspace Settings</h2>
        <button onClick={() => props.onClose()} class={styles.closeButton}>&times;</button>
      </header>

      <Show when={error()}>
        <div class={styles.error}>{error()}</div>
      </Show>

      <section class={styles.section}>
        <h3>Members</h3>
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
                    <Show when={props.user.role === 'admin' && member.id !== props.user.id} fallback={member.role}>
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
      </section>

      <section class={styles.section}>
        <h3>Pending Invitations</h3>
        <Show when={invitations().length > 0} fallback={<p>No pending invitations.</p>}>
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

        <Show when={props.user.role === 'admin'}>
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
                onChange={(e) => setInviteRole(e.target.value as 'admin' | 'editor' | 'viewer')}
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
        </Show>
      </section>
    </div>
  );
}
