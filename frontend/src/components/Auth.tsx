import { createSignal, Show } from 'solid-js';
import styles from './Auth.module.css';
import type { UserResponse, LoginResponse, LoginRequest, RegisterRequest, ErrorResponse } from '../types';
import { useI18n } from '../i18n';

interface AuthProps {
  onLogin: (token: string, user: UserResponse) => void;
}

export default function Auth(props: AuthProps) {
  const { t } = useI18n();
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

    const endpoint = isRegister() ? '/api/auth/register' : '/api/auth/login';
    const body: LoginRequest | RegisterRequest = isRegister()
      ? { email: email(), password: password(), name: name() }
      : { email: email(), password: password() };

    try {
      const res = await fetch(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });

      const data = (await res.json()) as LoginResponse;

      if (!res.ok) {
        const errorData = data as unknown as ErrorResponse;
        setError(errorData.error?.message || 'Authentication failed');
        return;
      }

      if (data.token && data.user) {
        props.onLogin(data.token, data.user);
      } else {
        setError('Invalid response from server');
      }
    } catch {
      setError('An error occurred. Please try again.');
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

        <div class={styles.oauthSection}>
          <div class={styles.divider}>
            <span>{t('auth.or')}</span>
          </div>
          <div class={styles.oauthButtons}>
            <a href="/api/auth/oauth/google" class={styles.googleButton}>
              {t('auth.loginWithGoogle')}
            </a>
            <a href="/api/auth/oauth/microsoft" class={styles.microsoftButton}>
              {t('auth.loginWithMicrosoft')}
            </a>
          </div>
        </div>

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
