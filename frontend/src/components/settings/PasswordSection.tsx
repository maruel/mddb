// Password management section for adding or changing password.

import { createSignal, Show } from 'solid-js';
import { useAuth } from '../../contexts';
import { useI18n } from '../../i18n';
import styles from './PasswordSection.module.css';

interface Props {
  hasPassword: boolean;
  onSuccess: (message: string) => void;
  onError: (message: string) => void;
}

export default function PasswordSection(props: Props) {
  const { t } = useI18n();
  const { api } = useAuth();

  const [currentPassword, setCurrentPassword] = createSignal('');
  const [newPassword, setNewPassword] = createSignal('');
  const [confirmPassword, setConfirmPassword] = createSignal('');
  const [loading, setLoading] = createSignal(false);

  const handleSubmit = async (e: Event) => {
    e.preventDefault();

    // Validate passwords match
    if (newPassword() !== confirmPassword()) {
      props.onError(t('settings.passwordMismatch'));
      return;
    }

    setLoading(true);
    try {
      await api().auth.setPassword({
        current_password: props.hasPassword ? currentPassword() : undefined,
        new_password: newPassword(),
      });

      // Clear form
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');

      props.onSuccess(props.hasPassword ? t('settings.passwordChanged') : t('settings.passwordAdded'));

      // Reload to update has_password state
      window.location.reload();
    } catch (err) {
      props.onError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div class={styles.section}>
      <h3>{props.hasPassword ? t('settings.changePassword') : t('settings.addPassword')}</h3>
      <p class={styles.hint}>{props.hasPassword ? t('settings.changePasswordHint') : t('settings.addPasswordHint')}</p>

      <form onSubmit={handleSubmit} class={styles.form}>
        <Show when={props.hasPassword}>
          <div class={styles.formItem}>
            <label>{t('settings.currentPassword')}</label>
            <input
              type="password"
              value={currentPassword()}
              onInput={(e) => setCurrentPassword(e.currentTarget.value)}
              required={props.hasPassword}
              autocomplete="current-password"
            />
          </div>
        </Show>

        <div class={styles.formItem}>
          <label>{t('settings.newPassword')}</label>
          <input
            type="password"
            value={newPassword()}
            onInput={(e) => setNewPassword(e.currentTarget.value)}
            required
            minLength={8}
            autocomplete="new-password"
          />
        </div>

        <div class={styles.formItem}>
          <label>{t('settings.confirmPassword')}</label>
          <input
            type="password"
            value={confirmPassword()}
            onInput={(e) => setConfirmPassword(e.currentTarget.value)}
            required
            minLength={8}
            autocomplete="new-password"
          />
        </div>

        <button type="submit" class={styles.submitButton} disabled={loading()}>
          {loading() ? '...' : props.hasPassword ? t('settings.changePassword') : t('settings.addPassword')}
        </button>
      </form>
    </div>
  );
}
