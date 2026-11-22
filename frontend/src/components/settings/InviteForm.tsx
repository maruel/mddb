// Creates recipient-bound or bearer invitation links and provides copying and revocation.

import { createSignal, For, Show } from "solid-js";
import PersonAddIcon from "@material-symbols/svg-400/outlined/person_add.svg?solid";
import CopyIcon from "@material-symbols/svg-400/outlined/content_copy.svg?solid";
import CloseIcon from "@material-symbols/svg-400/outlined/close.svg?solid";
import { IconButton } from "../shared";
import { useI18n } from "../../i18n";
import styles from "./InviteForm.module.css";

interface RoleOption<T extends string> {
  value: T;
  label: string;
}

interface PendingInvitation {
  id: string;
  email: string;
  role: string;
  invite_url: string;
  expires_at: number;
}

interface InviteFormProps<T extends string> {
  roleOptions: readonly RoleOption<T>[];
  defaultRole: T;
  pendingInvitations: PendingInvitation[];
  onInvite: (email: string, role: T) => Promise<boolean>;
  onRevoke: (id: string) => Promise<void>;
  loading?: boolean;
}

export default function InviteForm<T extends string>(props: InviteFormProps<T>) {
  const { t } = useI18n();
  const [email, setEmail] = createSignal("");
  const [role, setRole] = createSignal<T>(props.defaultRole);
  const [copied, setCopied] = createSignal<string | null>(null);
  const [copyError, setCopyError] = createSignal<string | null>(null);

  const selectRole = (value: string) => {
    const option = props.roleOptions.find((item) => item.value === value);
    if (option) setRole(() => option.value);
  };

  const handleSubmit = async (e: Event) => {
    e.preventDefault();
    if (await props.onInvite(email().trim(), role())) setEmail("");
  };

  const copy = async (invitation: PendingInvitation) => {
    setCopied(null);
    setCopyError(null);
    if (typeof navigator.clipboard === "undefined") {
      setCopyError(invitation.id);
      return;
    }
    try {
      await navigator.clipboard.writeText(invitation.invite_url);
      setCopied(invitation.id);
    } catch {
      setCopyError(invitation.id);
    }
  };

  return (
    <div class={styles.inviteSection}>
      <form onSubmit={handleSubmit} class={styles.inviteForm}>
        <h4>{t("settings.inviteNewMember")}</h4>
        <div class={styles.formGroup}>
          <label class={styles.recipient}>
            {t("invitations.recipient")}
            <input type="email" value={email()} onInput={(e) => setEmail(e.target.value)} disabled={props.loading} />
          </label>
          <label class={styles.role}>
            {t("settings.roleColumn")}
            <select value={role()} onChange={(e) => selectRole(e.target.value)} disabled={props.loading}>
              <For each={props.roleOptions}>{(option) => <option value={option.value}>{option.label}</option>}</For>
            </select>
          </label>
          <IconButton
            variant="unstyled"
            type="submit"
            disabled={props.loading}
            aria-label={t("invitations.createLink")}
            title={t("invitations.createLink")}
          >
            <PersonAddIcon aria-hidden="true" />
          </IconButton>
        </div>
        <p class={styles.hint}>{t("invitations.recipientHint")}</p>
      </form>

      <Show when={props.pendingInvitations.length > 0}>
        <section class={styles.pendingSection} aria-label={t("settings.pendingInvitations")}>
          <h4>{t("settings.pendingInvitations")}</h4>
          <ul class={styles.pendingList}>
            <For each={props.pendingInvitations}>
              {(inv) => (
                <li class={styles.invitation}>
                  <div class={styles.invitationHeading}>
                    <strong>{inv.email || t("invitations.anyoneWithLink")}</strong>
                    <span>{props.roleOptions.find((option) => option.value === inv.role)?.label || inv.role}</span>
                    <span>
                      {t("invitations.expires", { date: new Date(inv.expires_at * 1000).toLocaleDateString() })}
                    </span>
                  </div>
                  <div class={styles.linkRow}>
                    <input
                      class={styles.linkInput}
                      aria-label={t("invitations.link")}
                      value={inv.invite_url}
                      readOnly
                      onFocus={(e) => e.currentTarget.select()}
                      onClick={(e) => e.currentTarget.select()}
                    />
                    <IconButton
                      variant="unstyled"
                      type="button"
                      onClick={() => void copy(inv)}
                      aria-label={t("invitations.copyLink")}
                      title={t("invitations.copyLink")}
                    >
                      <CopyIcon aria-hidden="true" />
                    </IconButton>
                    <IconButton
                      variant="unstyled"
                      type="button"
                      onClick={() => void props.onRevoke(inv.id)}
                      disabled={props.loading}
                      aria-label={t("invitations.revoke")}
                      title={t("invitations.revoke")}
                    >
                      <CloseIcon aria-hidden="true" />
                    </IconButton>
                  </div>
                  <Show when={copied() === inv.id}>
                    <p role="status" class={styles.hint}>
                      {t("invitations.copied")}
                    </p>
                  </Show>
                  <Show when={copyError() === inv.id}>
                    <p role="alert" class={styles.hint}>
                      {t("invitations.copyManually")}
                    </p>
                  </Show>
                </li>
              )}
            </For>
          </ul>
        </section>
      </Show>
    </div>
  );
}
