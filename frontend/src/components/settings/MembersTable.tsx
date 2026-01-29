// Shared members table component for workspace and organization settings.

import { For, Show } from 'solid-js';
import { useI18n } from '../../i18n';
import type { UserResponse } from '@sdk/types.gen';
import styles from './MembersTable.module.css';

interface RoleOption {
  value: string;
  label: string;
}

interface MembersTableProps {
  members: UserResponse[];
  currentUserId: string;
  roleOptions: RoleOption[];
  roleField: 'workspace_role' | 'org_role';
  onUpdateRole: (userId: string, role: string) => void;
  loading?: boolean;
}

export default function MembersTable(props: MembersTableProps) {
  const { t } = useI18n();

  const getMemberRole = (member: UserResponse): string => {
    return (props.roleField === 'workspace_role' ? member.workspace_role : member.org_role) || '';
  };

  return (
    <table class={styles.table}>
      <thead>
        <tr>
          <th>{t('settings.nameColumn')}</th>
          <th>{t('settings.emailColumn')}</th>
          <th>{t('settings.roleColumn')}</th>
        </tr>
      </thead>
      <tbody>
        <For each={props.members}>
          {(member) => (
            <tr>
              <td>{member.name}</td>
              <td>{member.email}</td>
              <td>
                <Show when={member.id !== props.currentUserId} fallback={getMemberRole(member)}>
                  <select
                    value={getMemberRole(member)}
                    onChange={(e) => props.onUpdateRole(member.id, e.target.value)}
                    class={styles.roleSelect}
                    disabled={props.loading}
                  >
                    <For each={props.roleOptions}>
                      {(option) => <option value={option.value}>{option.label}</option>}
                    </For>
                  </select>
                </Show>
              </td>
            </tr>
          )}
        </For>
      </tbody>
    </table>
  );
}
