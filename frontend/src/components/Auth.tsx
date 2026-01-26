// Authentication component handling login and registration forms.

import { createSignal, createResource, Show, For } from 'solid-js';
import { createAPIClient, APIError } from '../api.gen';
import type { UserResponse, OAuthProvider } from '../types.gen';
import { OAuthProviderGoogle, OAuthProviderMicrosoft, OAuthProviderGitHub } from '../types.gen';
import styles from './Auth.module.css';
import { useI18n } from '../i18n';

interface AuthProps {
  onLogin: (token: string, user: UserResponse) => void;
}

// Provider-specific configuration for OAuth buttons
const providerConfig: Record<OAuthProvider, { style: string; labelKey: string }> = {
  [OAuthProviderGoogle]: { style: styles.googleButton ?? '', labelKey: 'auth.loginWithGoogle' },
  [OAuthProviderMicrosoft]: { style: styles.microsoftButton ?? '', labelKey: 'auth.loginWithMicrosoft' },
  [OAuthProviderGitHub]: { style: styles.githubButton ?? '', labelKey: 'auth.loginWithGitHub' },
};

function getProviderConfig(provider: OAuthProvider): { style: string; label: string } {
  const config = providerConfig[provider];
  if (config) {
    return { style: config.style, label: config.labelKey };
  }
  // Fallback for unknown providers: use generic style and capitalize provider name
  return { style: styles.oauthButton ?? '', label: provider.charAt(0).toUpperCase() + provider.slice(1) };
}

// Create an unauthenticated API client for login/register
const api = createAPIClient((url, init) => fetch(url, init));

export default function Auth(props: AuthProps) {
  const { t } = useI18n();
  const [providers] = createResource(() => api.auth.listProviders().then((r) => r.providers));
  const [isRegister, setIsRegister] = createSignal(false);
  const [email, setEmail] = createSignal('');
  const [password, setPassword] = createSignal('');
  const [name, setName] = createSignal('');
  const [error, setError] = createSignal<string | null>(null);
  const [loading, setLoading] = createSignal(false);

  const handleSubmit = async (e: Event) => {
    e.preventDefault();
    setError(null);
    setLoading(true);

    try {
      const data = isRegister()
        ? await api.auth.register({ email: email(), password: password(), name: name() })
        : await api.auth.login({ email: email(), password: password() });

      if (data.token && data.user) {
        props.onLogin(data.token, data.user);
      } else {
        setError('Invalid response from server');
      }
    } catch (err) {
      if (err instanceof APIError) {
        setError(err.response.error?.message || 'Authentication failed');
      } else {
        setError('An error occurred. Please try again.');
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <div class={styles.authContainer}>
      <form class={styles.authForm} onSubmit={handleSubmit}>
        <h2>{isRegister() ? t('auth.createAccount') : t('auth.loginTitle')}</h2>

        {error() && <div class={styles.error}>{error()}</div>}

        <Show when={isRegister()}>
          <div class={styles.formGroup}>
            <label for="name">{t('auth.name')}</label>
            <input
              id="name"
              name="name"
              type="text"
              value={name()}
              onInput={(e) => setName(e.target.value)}
              required
              autocomplete="name"
            />
          </div>
        </Show>

        <div class={styles.formGroup}>
          <label for="email">{t('auth.email')}</label>
          <input
            id="email"
            name="email"
            type="email"
            value={email()}
            onInput={(e) => setEmail(e.target.value)}
            required
            autocomplete="email"
          />
        </div>

        <div class={styles.formGroup}>
          <label for="password">{t('auth.password')}</label>
          <input
            id="password"
            name="password"
            type="password"
            value={password()}
            onInput={(e) => setPassword(e.target.value)}
            required
            autocomplete={isRegister() ? 'new-password' : 'current-password'}
          />
        </div>

        <button type="submit" disabled={loading()}>
          {loading() ? t('auth.pleaseWait') : isRegister() ? t('auth.register') : t('auth.login')}
        </button>

        <Show when={(providers() ?? []).length > 0}>
          <div class={styles.oauthSection}>
            <div class={styles.divider}>
              <span>{t('auth.or')}</span>
            </div>
            <div class={styles.oauthButtons}>
              <For each={providers()}>
                {(provider) => {
                  const config = getProviderConfig(provider);
                  const isKnown = provider in providerConfig;
                  return (
                    <a href={`/api/auth/oauth/${provider}`} class={config.style}>
                      {isKnown ? t(config.label) : config.label}
                    </a>
                  );
                }}
              </For>
            </div>
          </div>
        </Show>

        <p class={styles.toggle}>
          {isRegister() ? t('auth.alreadyHaveAccount') : t('auth.dontHaveAccount')}
          <button type="button" onClick={() => setIsRegister(!isRegister())}>
            {isRegister() ? t('auth.login') : t('auth.register')}
          </button>
        </p>

        <div class={styles.authFooter}>
          <a
            href="/privacy"
            onClick={(e) => {
              e.preventDefault();
              window.history.pushState(null, '', '/privacy');
              window.dispatchEvent(new PopStateEvent('popstate'));
            }}
          >
            {t('app.privacyPolicy')}
          </a>
        </div>
      </form>
    </div>
  );
}
