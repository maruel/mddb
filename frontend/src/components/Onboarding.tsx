import { createSignal, Show, untrack } from 'solid-js';
import type { UserResponse } from '../types';
import styles from './Onboarding.module.css';
import { useI18n } from '../i18n';

interface OnboardingProps {
  user: UserResponse;
  token: string;
  onComplete: () => void;
}

export default function Onboarding(props: OnboardingProps) {
  const { t } = useI18n();
  const [step, setStep] = createSignal(untrack(() => props.user.onboarding?.step || 'name'));
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);

  const [orgName, setOrgName] = createSignal(untrack(() => props.user.memberships?.[0]?.organization_name || ''));

  // Git states
  const [remoteURL, setRemoteURL] = createSignal('');
  const [remoteToken, setRemoteToken] = createSignal('');

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
    return res;
  };

  const updateStep = async (nextStep: string, completed = false) => {
    try {
      setLoading(true);
      setError(null);

      const res = await authFetch('/api/onboarding', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          state: {
            completed,
            step: nextStep,
            updated_at: new Date().toISOString(),
          },
        }),
      });

      if (!res.ok) throw new Error('Failed to update onboarding state');

      if (completed) {
        props.onComplete();
      } else {
        setStep(nextStep);
      }
    } catch (err) {
      setError('Error: ' + err);
    } finally {
      setLoading(false);
    }
  };

  const handleNameStep = async () => {
    try {
      setLoading(true);
      setError(null);

      // Update organization name
      const res = await authFetch('/api/settings/organization', {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: orgName() }),
      });

      if (!res.ok) throw new Error('Failed to update organization name');

      // Proceed to next step
      await updateStep('git');
    } catch (err) {
      setError('Error: ' + err);
    } finally {
      setLoading(false);
    }
  };

  const handleSkipGit = () => {
    updateStep('done', true);
  };

  const handleSetupGit = async (e: Event) => {
    e.preventDefault();
    try {
      setLoading(true);
      const res = await authFetch('/api/settings/git/remotes', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: 'origin',
          url: remoteURL(),
          token: remoteToken(),
          type: 'custom',
          auth_type: remoteToken() ? 'token' : 'none',
        }),
      });

      if (!res.ok) throw new Error('Failed to setup git remote');

      updateStep('done', true);
    } catch (err) {
      setError('Git setup failed: ' + err);
    } finally {
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
          <Show when={step() === 'name'}>
            <div class={styles.step}>
              <h3>{t('onboarding.confirmWorkspaceName')}</h3>
              <div class={styles.formGroup}>
                <label>{t('onboarding.workspaceName')}</label>
                <input type="text" value={orgName()} onInput={(e) => setOrgName(e.target.value)} />
                <p class={styles.hint}>{t('onboarding.workspaceNameHint')}</p>
              </div>
              <button class={styles.primaryButton} onClick={handleNameStep} disabled={!orgName() || loading()}>
                {loading() ? t('common.saving') : t('onboarding.nextGitSync')}
              </button>
            </div>
          </Show>

          <Show when={step() === 'git'}>
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
                  <button type="button" class={styles.secondaryButton} onClick={handleSkipGit}>
                    {t('onboarding.skipForNow')}
                  </button>
                  <button type="submit" class={styles.primaryButton} disabled={!remoteURL() || loading()}>
                    {loading() ? t('common.saving') : t('onboarding.setupAndFinish')}
                  </button>
                </div>
              </form>
            </div>
          </Show>
        </div>

        <div class={styles.footer}>
          <div class={styles.progress}>
            <div class={`${styles.dot} ${step() === 'name' ? styles.active : ''}`} />
            <div class={`${styles.dot} ${step() === 'git' ? styles.active : ''}`} />
          </div>
        </div>
      </div>
    </div>
  );
}
