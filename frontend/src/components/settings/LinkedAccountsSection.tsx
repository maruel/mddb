// Linked accounts section for managing OAuth provider connections.

import { createSignal, createEffect, Show, For } from 'solid-js';
import type { OAuthIdentity, OAuthProvider, LinkOAuthAccountResponse, ErrorResponse } from '@sdk/types.gen';
import { OAuthProviderGoogle, OAuthProviderMicrosoft, OAuthProviderGitHub } from '@sdk/types.gen';
import { useAuth } from '../../contexts';
import { useI18n } from '../../i18n';
import styles from './LinkedAccountsSection.module.css';

import PublicIcon from '@material-symbols/svg-400/outlined/public.svg?solid';

interface Props {
  oauthIdentities: OAuthIdentity[] | undefined;
  hasPassword: boolean;
  onSuccess: (message: string) => void;
  onError: (message: string) => void;
}

interface ProviderInfo {
  id: OAuthProvider;
  name: string;
  icon: SolidSVG;
}

const PROVIDERS: ProviderInfo[] = [
  { id: OAuthProviderGoogle, name: 'Google', icon: PublicIcon },
  { id: OAuthProviderMicrosoft, name: 'Microsoft', icon: PublicIcon },
  { id: OAuthProviderGitHub, name: 'GitHub', icon: PublicIcon },
];

export default function LinkedAccountsSection(props: Props) {
  const { t } = useI18n();
  const { api, token } = useAuth();

  const [availableProviders, setAvailableProviders] = createSignal<OAuthProvider[]>([]);
  const [loading, setLoading] = createSignal<OAuthProvider | null>(null);

  // Helper to make authenticated fetch calls for OAuth endpoints (not in generated SDK)
  const authFetch = async (url: string, body: object) => {
    const res = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token()}`,
      },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      const error = (await res.json()) as ErrorResponse;
      throw new Error(error.error.message);
    }
    return res.json();
  };

  // Fetch available providers on mount
  createEffect(() => {
    api()
      .auth.listProviders()
      .then((resp) => setAvailableProviders(resp.providers))
      .catch((err) => console.error('Failed to fetch providers:', err));
  });

  // Check if a provider is linked
  const isLinked = (providerId: OAuthProvider): OAuthIdentity | undefined => {
    return props.oauthIdentities?.find((id) => id.provider === providerId);
  };

  // Check if user can unlink (must have another auth method)
  const canUnlink = (): boolean => {
    const linkedCount = props.oauthIdentities?.length || 0;
    return props.hasPassword || linkedCount > 1;
  };

  // Link a provider
  const handleLink = async (providerId: OAuthProvider) => {
    setLoading(providerId);
    try {
      const resp = (await authFetch('/api/v1/auth/oauth/link', { provider: providerId })) as LinkOAuthAccountResponse;
      // Redirect to OAuth provider
      window.location.href = resp.redirect_url;
    } catch (err) {
      props.onError(`${t('settings.linkingFailed')}: ${err}`);
      setLoading(null);
    }
  };

  // Unlink a provider
  const handleUnlink = async (providerId: OAuthProvider) => {
    if (!canUnlink()) {
      props.onError(t('settings.cannotUnlinkOnly'));
      return;
    }

    setLoading(providerId);
    try {
      await authFetch('/api/v1/auth/oauth/unlink', { provider: providerId });
      props.onSuccess(t('settings.accountUnlinked'));
      // Refresh user data
      window.location.reload();
    } catch (err) {
      props.onError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setLoading(null);
    }
  };

  return (
    <div class={styles.section}>
      <h3>{t('settings.linkedAccounts')}</h3>
      <p class={styles.hint}>{t('settings.linkedAccountsHint')}</p>

      <div class={styles.providerList}>
        <For each={PROVIDERS}>
          {(provider) => {
            const linked = () => isLinked(provider.id);
            const isAvailable = () => availableProviders().includes(provider.id);
            const isLoading = () => loading() === provider.id;

            return (
              <Show when={isAvailable()}>
                <div class={styles.providerRow}>
                  <div class={styles.providerInfo}>
                    <span class={styles.providerIcon}>
                      <provider.icon />
                    </span>
                    <span class={styles.providerName}>{provider.name}</span>
                    <Show when={linked()} keyed>
                      {(identity) => (
                        <>
                          <span class={styles.linkedBadge}>{t('settings.linked')}</span>
                          <span class={styles.linkedEmail}>{identity.email}</span>
                        </>
                      )}
                    </Show>
                    <Show when={!linked()}>
                      <span class={styles.notLinkedBadge}>{t('settings.notLinked')}</span>
                    </Show>
                  </div>
                  <div class={styles.providerActions}>
                    <Show
                      when={linked()}
                      fallback={
                        <button
                          class={styles.linkButton}
                          onClick={() => handleLink(provider.id)}
                          disabled={isLoading()}
                        >
                          {isLoading() ? '...' : t('settings.linkAccount')}
                        </button>
                      }
                    >
                      <button
                        class={styles.unlinkButton}
                        onClick={() => handleUnlink(provider.id)}
                        disabled={isLoading() || !canUnlink()}
                        title={!canUnlink() ? t('settings.cannotUnlinkOnly') : ''}
                      >
                        {isLoading() ? '...' : t('settings.unlinkAccount')}
                      </button>
                    </Show>
                  </div>
                </div>
              </Show>
            );
          }}
        </For>
      </div>
    </div>
  );
}
