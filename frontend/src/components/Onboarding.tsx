import { createSignal, createMemo, Show, untrack } from 'solid-js';
import { createApi } from '../useApi';
import type { UserResponse } from '../types.gen';
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

  // Create API client
  const api = createMemo(() => createApi(() => props.token));
  const orgApi = createMemo(() => {
    const orgID = props.user.organization_id;
    return orgID ? api().org(orgID) : null;
  });

  const updateStep = async (nextStep: string, completed = false) => {
    const org = orgApi();
    if (!org) return;

    try {
      setLoading(true);
      setError(null);

      await org.onboarding.update({
        state: {
          completed,
          step: nextStep,
          updated_at: new Date().toISOString(),
        },
      });

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
    const org = orgApi();
    if (!org) return;

    try {
      setLoading(true);
      setError(null);

      // Update organization name
      await org.settings.organization.update({ name: orgName() });

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
    const org = orgApi();
    if (!org) return;

    try {
      setLoading(true);
      await org.settings.git.update({
        url: remoteURL(),
        token: remoteToken(),
        type: 'custom',
        auth_type: remoteToken() ? 'token' : 'none',
      });

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
