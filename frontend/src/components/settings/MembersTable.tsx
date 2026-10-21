// Responsive member rows with role editing and removal for workspace and organization settings.

import { For, Show } from "solid-js";
import PersonRemoveIcon from "@material-symbols/svg-400/outlined/person_remove.svg?solid";
import { IconButton } from "../shared";
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
    <div class={styles.container}>
      <table class={styles.table} role="table">
        <thead role="rowgroup">
          <tr role="row">
            <th scope="col" role="columnheader">
              {t("settings.nameColumn")}
            </th>
            <th scope="col" role="columnheader">
              {t("settings.emailColumn")}
            </th>
            <th scope="col" role="columnheader">
              {t("settings.roleColumn")}
            </th>
            <Show when={props.onRemove}>
              <th scope="col" role="columnheader">
                {t("settings.actionsColumn")}
              </th>
            </Show>
          </tr>
        </thead>
        <tbody role="rowgroup">
          <For each={props.members}>
            {(member) => (
              <tr role="row">
                <td class={styles.name} role="cell">
                  <span class={styles.label} aria-hidden="true">
                    {t("settings.nameColumn")}
                  </span>
                  <span>{member.name}</span>
                </td>
                <td class={styles.email} role="cell">
                  <span class={styles.label} aria-hidden="true">
                    {t("settings.emailColumn")}
                  </span>
                  <span>{member.email}</span>
                </td>
                <td role="cell">
                  <span class={styles.label} aria-hidden="true">
                    {t("settings.roleColumn")}
                  </span>
                  <Show when={member.id !== props.currentUserId} fallback={getMemberRole(member)}>
                    <select
                      aria-label={`${t("settings.roleColumn")}: ${member.name}`}
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
                  <td role="cell">
                    <Show when={member.id !== props.currentUserId}>
                      <span class={styles.label} aria-hidden="true">
                        {t("settings.actionsColumn")}
                      </span>
                      <IconButton
                        variant="unstyled"
                        aria-label={t("common.remove")}
                        title={t("common.remove")}
                        class={styles.removeButton}
                        onClick={() => props.onRemove?.(member.id)}
                        disabled={props.loading}
                      >
                        <PersonRemoveIcon aria-hidden="true" />
                      </IconButton>
                    </Show>
                  </td>
                </Show>
              </tr>
            )}
          </For>
        </tbody>
      </table>
    </div>
  );
}
