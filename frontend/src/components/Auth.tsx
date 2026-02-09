// Authentication component handling login and registration forms.

import { createSignal, createResource, createMemo, Show, For, type JSX } from 'solid-js';
import { A } from '@solidjs/router';
import { createAPIClient, APIError } from '@sdk/api.gen';
import type { UserResponse, OAuthProvider } from '@sdk/types.gen';
import { OAuthProviderGoogle, OAuthProviderMicrosoft, OAuthProviderGitHub } from '@sdk/types.gen';
import styles from './Auth.module.css';
import { GoogleIcon, MicrosoftIcon, GitHubIcon } from './OAuthIcons';
import { useI18n } from '../i18n';

interface AuthProps {
  onLogin: (token: string, user: UserResponse) => void;
}

// Map providers to their icon components
const providerIcons: Record<OAuthProvider, (props: JSX.SvgSVGAttributes<SVGSVGElement>) => JSX.Element> = {
  [OAuthProviderGoogle]: GoogleIcon,
  [OAuthProviderMicrosoft]: MicrosoftIcon,
  [OAuthProviderGitHub]: GitHubIcon,
};

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
  const sortedProviders = createMemo(() => {
    const list = providers() ?? [];
    const order = [OAuthProviderGoogle, OAuthProviderGitHub, OAuthProviderMicrosoft];
    return [...list].sort((a, b) => {
      let ia = order.indexOf(a);
      let ib = order.indexOf(b);
      if (ia === -1) ia = order.length;
      if (ib === -1) ib = order.length;
      return ia - ib;
    });
  });
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

        <Show when={sortedProviders().length > 0}>
          <div class={styles.oauthButtons}>
            <For each={sortedProviders()}>
              {(provider) => {
                const config = getProviderConfig(provider);
                const isKnown = provider in providerConfig;
                const Icon = providerIcons[provider];
                return (
                  <a href={`/api/v1/auth/oauth/${provider}`} class={config.style} target="_self">
                    {Icon && <Icon class={styles.oauthIcon} />}
                    {isKnown ? t(config.label) : config.label}
                  </a>
                );
              }}
            </For>
          </div>
          <div class={styles.divider}>
            <span>{t('auth.or')}</span>
          </div>
        </Show>

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

        <p class={styles.toggle}>
          {isRegister() ? t('auth.alreadyHaveAccount') : t('auth.dontHaveAccount')}{' '}
          <button type="button" onClick={() => setIsRegister(!isRegister())}>
            {isRegister() ? t('auth.login') : t('auth.register')}
          </button>
        </p>

        <div class={styles.authFooter}>
          <A href="/privacy">{t('app.privacyPolicy')}</A>
        </div>
      </form>
    </div>
  );
}
