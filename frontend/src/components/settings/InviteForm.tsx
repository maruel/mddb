// Shared invite form component for workspace and organization settings.

import { createSignal, For, Show } from 'solid-js';
import { useI18n } from '../../i18n';
import styles from './InviteForm.module.css';

interface RoleOption {
  value: string;
  label: string;
}

interface PendingInvitation {
  email: string;
  role: string;
  created: string | number;
}

interface InviteFormProps {
  roleOptions: RoleOption[];
  defaultRole: string;
  pendingInvitations: PendingInvitation[];
  onInvite: (email: string, role: string) => Promise<void>;
  loading?: boolean;
}

export default function InviteForm(props: InviteFormProps) {
  const { t } = useI18n();
  const [email, setEmail] = createSignal('');
  const [role, setRole] = createSignal(props.defaultRole);

  const handleSubmit = async (e: Event) => {
    e.preventDefault();
    if (!email()) return;
    await props.onInvite(email(), role());
    setEmail('');
  };

  return (
    <div class={styles.inviteSection}>
      <form onSubmit={handleSubmit} class={styles.inviteForm}>
        <h4>{t('settings.inviteNewMember')}</h4>
        <div class={styles.formGroup}>
          <input
            type="email"
            placeholder={t('settings.emailPlaceholder') || 'Email address'}
            value={email()}
            onInput={(e) => setEmail(e.target.value)}
            required
            disabled={props.loading}
          />
          <select value={role()} onChange={(e) => setRole(e.target.value)} disabled={props.loading}>
            <For each={props.roleOptions}>{(option) => <option value={option.value}>{option.label}</option>}</For>
          </select>
          <button type="submit" disabled={props.loading}>
            {t('common.invite')}
          </button>
        </div>
      </form>

      <Show when={props.pendingInvitations.length > 0}>
        <div class={styles.pendingSection}>
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
              <For each={props.pendingInvitations}>
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
        </div>
      </Show>
    </div>
  );
}
