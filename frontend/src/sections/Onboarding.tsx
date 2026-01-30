// Onboarding component for first-time users without org/workspace.

import { createEffect, createSignal, Show } from 'solid-js';
import { useNavigate } from '@solidjs/router';
import { useAuth } from '../contexts';
import { useI18n } from '../i18n';
import { workspaceUrl } from '../utils/urls';

/**
 * Onboarding handles the first-login flow for users who have authenticated
 * but don't yet have an organization or workspace.
 *
 * Flow:
 * 1. Check if user has no organizations -> create one
 * 2. Check if user has no workspaces -> create one
 * 3. Redirect to the new workspace
 */
export default function Onboarding() {
  const { t, ready: i18nReady } = useI18n();
  const { user, api, setUser } = useAuth();
  const navigate = useNavigate();

  const [status, setStatus] = createSignal<'loading' | 'creating-org' | 'creating-ws' | 'done'>('loading');
  const [error, setError] = createSignal<string | null>(null);

  // Get user's first name for default naming
  function getUserFirstName(): string {
    const u = user();
    if (!u?.name) return '';
    const firstName = u.name.split(' ')[0];
    return firstName || u.name;
  }

  // Track whether we're already running to prevent duplicate executions
  const [running, setRunning] = createSignal(false);

  // Run first-login flow
  createEffect(() => {
    const u = user();
    if (!u || !i18nReady() || running()) return;

    // If user already has a workspace, redirect
    if (u.workspace_id) {
      navigate(workspaceUrl(u.workspace_id, u.workspace_name), { replace: true });
      return;
    }

    // Run async flow
    setRunning(true);
    (async () => {
      try {
        const orgs = u.organizations || [];

        // Step 1: Create organization if needed
        if (orgs.length === 0) {
          setStatus('creating-org');
          const firstName = getUserFirstName();
          const orgName = firstName
            ? (t('onboarding.defaultOrgName', { name: firstName }) as string)
            : (t('onboarding.defaultOrgNameFallback') as string);

          await api().organizations.createOrganization({ name: orgName });

          // Refresh user data - effect will re-run with updated user
          const updatedUser = await api().auth.getMe();
          setUser(updatedUser);
          setRunning(false);
          return;
        }

        // Step 2: Create workspace if needed
        const firstOrg = orgs[0];
        if (firstOrg) {
          const orgWorkspaces = u.workspaces?.filter((ws) => ws.organization_id === firstOrg.organization_id) || [];
          if (orgWorkspaces.length === 0) {
            setStatus('creating-ws');
            const firstName = getUserFirstName();
            const wsName = firstName
              ? (t('onboarding.defaultWorkspaceName', { name: firstName }) as string)
              : (t('onboarding.defaultWorkspaceNameFallback') as string);

            const ws = await api().org(firstOrg.organization_id).workspaces.createWorkspace({ name: wsName });

            // Switch to the new workspace - effect will re-run with updated user
            const switchResult = await api().auth.switchWorkspace({ ws_id: ws.id });
            if (switchResult.user) {
              setUser(switchResult.user);
            }
            setRunning(false);
            return;
          }
        }

        // Step 3: User has org and workspaces but no active workspace - pick first one
        if (!u.workspace_id && u.workspaces && u.workspaces.length > 0) {
          const firstWs = u.workspaces[0];
          if (firstWs) {
            const switchResult = await api().auth.switchWorkspace({ ws_id: firstWs.workspace_id });
            if (switchResult.user) {
              setUser(switchResult.user);
            }
          }
        }

        setStatus('done');
        setRunning(false);
      } catch (err) {
        console.error('Onboarding error:', err);
        setError(String(err));
        setRunning(false);
      }
    })();
  });

  return (
    <div
      style={{
        display: 'flex',
        'flex-direction': 'column',
        'align-items': 'center',
        'justify-content': 'center',
        height: '100vh',
        padding: '2rem',
        'text-align': 'center',
      }}
    >
      <Show when={error()}>
        <div style={{ color: 'var(--c-error, red)', 'margin-bottom': '1rem' }}>{error()}</div>
      </Show>
      <Show when={!error()}>
        <div style={{ 'font-size': '1.5rem', 'margin-bottom': '1rem' }}>
          {status() === 'creating-org' && (t('onboarding.creatingOrg') || 'Creating your organization...')}
          {status() === 'creating-ws' && (t('onboarding.creatingWorkspace') || 'Creating your workspace...')}
          {(status() === 'loading' || status() === 'done') && (t('common.loading') || 'Loading...')}
        </div>
        <div
          style={{
            width: '40px',
            height: '40px',
            border: '3px solid var(--c-border, #ccc)',
            'border-top-color': 'var(--c-primary, #007bff)',
            'border-radius': '50%',
            animation: 'spin 1s linear infinite',
          }}
        />
        <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
      </Show>
    </div>
  );
}
