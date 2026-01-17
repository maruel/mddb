import { createSignal, Show } from 'solid-js';
import styles from './Auth.module.css';
import type { User, LoginResponse, LoginRequest, RegisterRequest, ErrorResponse } from '../types';

interface AuthProps {
  onLogin: (token: string, user: User) => void;
}

export default function Auth(props: AuthProps) {
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
        const errorData = (data as unknown) as ErrorResponse;
        setError(errorData.error?.message || 'Authentication failed');
        return;
      }

      if (data.token && data.user) {
        props.onLogin(data.token, data.user);
      } else {
        setError('Invalid response from server');
      }
    } catch (err) {
      setError('An error occurred. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div class={styles.authContainer}>
      <form class={styles.authForm} onSubmit={handleSubmit}>
        <h2>{isRegister() ? 'Create Account' : 'Login to mddb'}</h2>
        
        {error() && <div class={styles.error}>{error()}</div>}

        <Show when={isRegister()}>
          <div class={styles.formGroup}>
            <label for="name">Name</label>
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
          <label for="email">Email</label>
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
          <label for="password">Password</label>
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
          {loading() ? 'Please wait...' : (isRegister() ? 'Register' : 'Login')}
        </button>

        <p class={styles.toggle}>
          {isRegister() ? 'Already have an account?' : "Don't have an account?"}
          <button type="button" onClick={() => setIsRegister(!isRegister())}>
            {isRegister() ? 'Login' : 'Register'}
          </button>
        </p>
      </form>
    </div>
  );
}
