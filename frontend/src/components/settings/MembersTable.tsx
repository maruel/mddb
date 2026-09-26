// Shared members table component for workspace and organization settings.

import { For, Show } from "solid-js";
import { useI18n } from "../../i18n";
import type { UserResponse } from "@sdk/types.gen";
import styles from "./MembersTable.module.css";

interface RoleOption<T extends string> {
  value: T;
  label: string;
}

interface MembersTableProps<T extends string> {
  members: UserResponse[];
  currentUserId: string;
  roleOptions: readonly RoleOption<T>[];
  roleField: "workspace_role" | "org_role";
  onUpdateRole: (userId: string, role: T) => void;
  onRemove?: (userId: string) => void;
  loading?: boolean;
}

export default function MembersTable<T extends string>(props: MembersTableProps<T>) {
  const { t } = useI18n();

  const getMemberRole = (member: UserResponse): string => {
    return (props.roleField === "workspace_role" ? member.workspace_role : member.org_role) || "";
  };

  const updateRole = (userId: string, value: string) => {
    const option = props.roleOptions.find((item) => item.value === value);
    if (option) props.onUpdateRole(userId, option.value);
  };

  return (
    <table class={styles.table}>
      <thead>
        <tr>
          <th>{t("settings.nameColumn")}</th>
          <th>{t("settings.emailColumn")}</th>
          <th>{t("settings.roleColumn")}</th>
          <Show when={props.onRemove}>
            <th>{t("settings.actionsColumn")}</th>
          </Show>
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
                    onChange={(e) => updateRole(member.id, e.target.value)}
                    class={styles.roleSelect}
                    disabled={props.loading}
                  >
                    <For each={props.roleOptions}>
                      {(option) => <option value={option.value}>{option.label}</option>}
                    </For>
                  </select>
                </Show>
              </td>
              <Show when={props.onRemove}>
                <td>
                  <Show when={member.id !== props.currentUserId}>
                    <button
                      class={styles.removeButton}
                      onClick={() => props.onRemove?.(member.id)}
                      disabled={props.loading}
                    >
                      {t("common.remove")}
                    </button>
                  </Show>
                </td>
              </Show>
            </tr>
          )}
        </For>
      </tbody>
    </table>
  );
}
