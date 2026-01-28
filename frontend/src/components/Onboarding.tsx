// Onboarding wizard for new users to configure their workspace.

import { createSignal, Show } from 'solid-js';
import { useAuth } from '../contexts';
import styles from './Onboarding.module.css';
import { useI18n } from '../i18n';

interface OnboardingProps {
  onComplete: () => void;
}

export default function Onboarding(props: OnboardingProps) {
  const { t } = useI18n();
  const { wsApi } = useAuth();

  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);

  // Git states
  const [remoteURL, setRemoteURL] = createSignal('');
  const [remoteToken, setRemoteToken] = createSignal('');

  const handleSkipGit = () => {
    props.onComplete();
  };

  const handleSetupGit = async (e: Event) => {
    e.preventDefault();
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      setError(null);

      // Setup git remote
      await ws.settings.git.updateGitRemote({
        url: remoteURL(),
        token: remoteToken(),
        type: 'custom',
        auth_type: remoteToken() ? 'token' : 'none',
      });

      props.onComplete();
    } catch (err) {
      setError('Git setup failed: ' + err);
      setLoading(false);
    }
  };

  return (
    <div class={styles.overlay}>
      <div class={styles.modal}>
        <header class={styles.header}>
          <h2>{t('onboarding.welcome')}</h2>
          <p>{t('onboarding.letsGetStarted')}</p>
        </header>

        <Show when={error()}>
          <div class={styles.error}>{error()}</div>
        </Show>

        <div class={styles.content}>
          <div class={styles.step}>
            <h3>{t('onboarding.advancedSyncTitle')}</h3>
            <p>{t('onboarding.advancedSyncHint')}</p>

            <form onSubmit={handleSetupGit} class={styles.gitForm}>
              <div class={styles.formGroup}>
                <label>{t('onboarding.repoUrl')}</label>
                <input
                  type="url"
                  placeholder={t('onboarding.repoUrlPlaceholder') || 'https://github.com/user/repo.git'}
                  value={remoteURL()}
                  onInput={(e) => setRemoteURL(e.target.value)}
                />
              </div>
              <div class={styles.formGroup}>
                <label>{t('onboarding.patLabel')}</label>
                <input
                  type="password"
                  placeholder={t('onboarding.patPlaceholder') || 'ghp_...'}
                  value={remoteToken()}
                  onInput={(e) => setRemoteToken(e.target.value)}
                />
              </div>
              <div class={styles.actions}>
                <button type="button" class={styles.secondaryButton} onClick={handleSkipGit} disabled={loading()}>
                  {t('onboarding.skipForNow')}
                </button>
                <button type="submit" class={styles.primaryButton} disabled={!remoteURL() || loading()}>
                  {loading() ? t('common.saving') : t('onboarding.setupAndFinish')}
                </button>
              </div>
            </form>
          </div>
        </div>
      </div>
    </div>
  );
}
