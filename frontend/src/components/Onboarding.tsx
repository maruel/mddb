import { createSignal, Show, untrack } from 'solid-js';
import type { User } from '../types';
import styles from './Onboarding.module.css';

interface OnboardingProps {
  user: User;
  token: string;
  onComplete: () => void;
}

export default function Onboarding(props: OnboardingProps) {
  const [step, setStep] = createSignal(untrack(() => props.user.onboarding?.step || 'name'));
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);

  const [orgName, setOrgName] = createSignal(
    untrack(() => props.user.memberships?.[0]?.organization_name || '')
  );

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
          <h2>Welcome to mddb!</h2>
          <p>Let's get your workspace ready.</p>
        </header>

        <Show when={error()}>
          <div class={styles.error}>{error()}</div>
        </Show>

        <div class={styles.content}>
          <Show when={step() === 'name'}>
            <div class={styles.step}>
              <h3>Confirm Workspace Name</h3>
              <div class={styles.formGroup}>
                <label>Workspace Name</label>
                <input
                  type="text"
                  value={orgName()}
                  onInput={(e) => setOrgName(e.target.value)}
                  disabled
                />
                <p class={styles.hint}>This is derived from your registration name.</p>
              </div>
              <button class={styles.primaryButton} onClick={() => updateStep('members')}>
                Next: Invite Team
              </button>
            </div>
          </Show>

          <Show when={step() === 'members'}>
            <div class={styles.step}>
              <h3>Team Collaboration</h3>
              <p>You can invite your team members now or later in settings.</p>
              <div class={styles.illustration}>👥</div>
              <button class={styles.primaryButton} onClick={() => updateStep('git')}>
                Next: Git Sync
              </button>
            </div>
          </Show>

          <Show when={step() === 'git'}>
            <div class={styles.step}>
              <h3>Advanced Sync (Optional)</h3>
              <p>Keep your data safe by synchronizing with a private Git repository.</p>

              <form onSubmit={handleSetupGit} class={styles.gitForm}>
                <div class={styles.formGroup}>
                  <label>GitHub/GitLab Repository URL</label>
                  <input
                    type="url"
                    placeholder="https://github.com/user/repo.git"
                    value={remoteURL()}
                    onInput={(e) => setRemoteURL(e.target.value)}
                  />
                </div>
                <div class={styles.formGroup}>
                  <label>Personal Access Token</label>
                  <input
                    type="password"
                    placeholder="ghp_..."
                    value={remoteToken()}
                    onInput={(e) => setRemoteToken(e.target.value)}
                  />
                </div>
                <div class={styles.actions}>
                  <button type="button" class={styles.secondaryButton} onClick={handleSkipGit}>
                    Skip for now
                  </button>
                  <button
                    type="submit"
                    class={styles.primaryButton}
                    disabled={!remoteURL() || loading()}
                  >
                    {loading() ? 'Saving...' : 'Setup & Finish'}
                  </button>
                </div>
              </form>
            </div>
          </Show>
        </div>

        <div class={styles.footer}>
          <div class={styles.progress}>
            <div classList={{ [styles.dot]: true, [styles.active]: step() === 'name' }} />
            <div classList={{ [styles.dot]: true, [styles.active]: step() === 'members' }} />
            <div classList={{ [styles.dot]: true, [styles.active]: step() === 'git' }} />
          </div>
        </div>
      </div>
    </div>
  );
}
