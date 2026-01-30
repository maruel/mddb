// Settings section with nested routes for user, workspace, org, and server settings.

import { lazy, Show, Suspense, type JSX } from 'solid-js';
import { Navigate, useParams, useLocation, useNavigate } from '@solidjs/router';
import { useAuth } from '../contexts';
import { useI18n } from '../i18n';
import { settingsUrl } from '../utils/urls';
import SettingsLayout from './SettingsLayout';

const ProfileSettings = lazy(() => import('../components/settings/ProfileSettings'));
const WorkspaceSettingsPanel = lazy(() => import('../components/settings/WorkspaceSettingsPanel'));
const OrgSettingsPanel = lazy(() => import('../components/settings/OrgSettingsPanel'));
const ServerSettingsPanel = lazy(() => import('../components/settings/ServerSettingsPanel'));

// Loading fallback
function SettingsLoading() {
  const { t } = useI18n();
  return <div style={{ padding: '2rem', color: 'var(--c-text-light)' }}>{t('common.loading')}</div>;
}

// Guard component for server settings (global admin only)
export function ServerSettingsGuard() {
  const { user } = useAuth();

  return (
    <Show when={user()?.is_global_admin} fallback={<Navigate href="/settings/user" />}>
      <Suspense fallback={<SettingsLoading />}>
        <ServerSettingsPanel />
      </Suspense>
    </Show>
  );
}

// Wrapper for WorkspaceSettingsPanel that extracts route params
export function WorkspaceSettingsRoute() {
  const params = useParams<{ wsId: string }>();
  const location = useLocation();
  const navigate = useNavigate();
  const { user } = useAuth();

  // Extract section from hash (e.g., #members, #settings, #sync)
  const section = () => location.hash?.slice(1) || undefined;

  // Extract wsId - handle slug format (id+name)
  const wsId = () => {
    const id = params.wsId;
    // Handle both "id" and "id+slug" formats
    return id?.split('+')[0] || '';
  };

  // Verify user has access to this workspace
  const hasAccess = () => {
    const u = user();
    return u?.workspaces?.some((ws) => ws.workspace_id === wsId());
  };

  const handleNavigateToOrgSettings = (orgId: string, orgName: string) => {
    navigate(settingsUrl('org', orgId, orgName));
  };

  return (
    <Show when={hasAccess()} fallback={<Navigate href="/settings/user" />}>
      <Suspense fallback={<SettingsLoading />}>
        <WorkspaceSettingsPanel
          wsId={wsId()}
          section={section()}
          onNavigateToOrgSettings={handleNavigateToOrgSettings}
        />
      </Suspense>
    </Show>
  );
}

// Wrapper for OrgSettingsPanel that extracts route params
export function OrgSettingsRoute() {
  const params = useParams<{ orgId: string }>();
  const location = useLocation();
  const { user } = useAuth();

  // Extract section from hash
  const section = () => location.hash?.slice(1) || undefined;

  // Extract orgId - handle slug format (id+name)
  const orgId = () => {
    const id = params.orgId;
    return id?.split('+')[0] || '';
  };

  // Verify user has access to this organization
  const hasAccess = () => {
    const u = user();
    return u?.organizations?.some((org) => org.organization_id === orgId());
  };

  return (
    <Show when={hasAccess()} fallback={<Navigate href="/settings/user" />}>
      <Suspense fallback={<SettingsLoading />}>
        <OrgSettingsPanel orgId={orgId()} section={section()} />
      </Suspense>
    </Show>
  );
}

// Profile settings with Suspense wrapper
export function ProfileSettingsRoute() {
  return (
    <Suspense fallback={<SettingsLoading />}>
      <ProfileSettings />
    </Suspense>
  );
}

// Redirect component for base /settings path
export function SettingsRedirect() {
  return <Navigate href="/settings/user" />;
}

interface SettingsSectionProps {
  children?: JSX.Element;
}

// Main settings section component - wraps children in layout
export default function SettingsSection(props: SettingsSectionProps) {
  return <SettingsLayout>{props.children}</SettingsLayout>;
}
