// Unified settings page container with sidebar navigation and content panels.

import { createSignal, createEffect, Show, onMount, onCleanup } from 'solid-js';
import { useAuth } from '../../contexts';
import { useI18n } from '../../i18n';
import type { UnifiedSettingsMatch } from '../../utils/urls';
import { settingsUrl, workspaceUrl } from '../../utils/urls';
import SettingsSidebar from './SettingsSidebar';
import ProfileSettings from './ProfileSettings';
import WorkspaceSettingsPanel from './WorkspaceSettingsPanel';
import OrgSettingsPanel from './OrgSettingsPanel';
import styles from './Settings.module.css';

interface SettingsProps {
  route: UnifiedSettingsMatch;
  onClose: () => void;
}

export default function Settings(props: SettingsProps) {
  const { t } = useI18n();
  const { user } = useAuth();
  const [showMobileSidebar, setShowMobileSidebar] = createSignal(false);

  // Handle Escape key to close mobile sidebar
  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Escape' && showMobileSidebar()) {
      setShowMobileSidebar(false);
    }
  };

  onMount(() => {
    window.addEventListener('keydown', handleKeyDown);
    onCleanup(() => window.removeEventListener('keydown', handleKeyDown));
  });

  // Handle redirect type
  createEffect(() => {
    if (props.route.type === 'redirect' && props.route.id) {
      window.history.replaceState(null, '', props.route.id);
      window.dispatchEvent(new PopStateEvent('popstate'));
    }
  });

  const navigateTo = (url: string) => {
    setShowMobileSidebar(false);
    window.history.pushState(null, '', url);
    window.dispatchEvent(new PopStateEvent('popstate'));
  };

  const handleBack = () => {
    window.history.back();
  };

  const navigateToWorkspace = () => {
    const wsId = user()?.workspace_id;
    const wsName = user()?.workspace_name;
    if (wsId) {
      props.onClose();
      window.history.pushState(null, '', workspaceUrl(wsId, wsName));
      window.dispatchEvent(new PopStateEvent('popstate'));
    }
  };

  // Get workspace and org info for navigation
  const getWorkspaceInfo = (wsId: string) => {
    const ws = user()?.workspaces?.find((w) => w.workspace_id === wsId);
    return ws ? { id: wsId, name: ws.workspace_name || wsId } : null;
  };

  const getOrgInfo = (orgId: string) => {
    const org = user()?.organizations?.find((o) => o.organization_id === orgId);
    return org ? { id: orgId, name: org.organization_name || orgId } : null;
  };

  return (
    <div class={styles.settingsPage}>
      <header class={styles.header}>
        <div class={styles.headerLeft}>
          <button
            class={styles.hamburger}
            onClick={() => setShowMobileSidebar(!showMobileSidebar())}
            aria-label="Toggle settings menu"
          >
            ☰
          </button>
          <button onClick={handleBack} class={styles.backButton}>
            ← {t('common.back')}
          </button>
          <h1 class={styles.title} onClick={navigateToWorkspace}>
            {t('settings.title')}
          </h1>
        </div>
      </header>

      <div class={styles.layout}>
        <Show when={showMobileSidebar()}>
          <div class={styles.mobileBackdrop} onClick={() => setShowMobileSidebar(false)} />
        </Show>

        <SettingsSidebar isOpen={showMobileSidebar()} currentRoute={props.route} onNavigate={navigateTo} />

        <main class={styles.content}>
          <Show when={props.route.type === 'profile'}>
            <ProfileSettings />
          </Show>

          <Show when={props.route.type === 'workspace' && props.route.id ? props.route.id : null}>
            {(wsId) => (
              <Show
                when={getWorkspaceInfo(wsId())}
                fallback={<div class={styles.error}>{t('errors.failedToLoad')}</div>}
              >
                <WorkspaceSettingsPanel
                  wsId={wsId()}
                  section={props.route.section}
                  onNavigateToOrgSettings={(orgId, orgName) => {
                    navigateTo(settingsUrl('org', orgId, orgName));
                  }}
                />
              </Show>
            )}
          </Show>

          <Show when={props.route.type === 'org' && props.route.id ? props.route.id : null}>
            {(orgId) => (
              <Show when={getOrgInfo(orgId())} fallback={<div class={styles.error}>{t('errors.failedToLoad')}</div>}>
                <OrgSettingsPanel orgId={orgId()} section={props.route.section} />
              </Show>
            )}
          </Show>
        </main>
      </div>
    </div>
  );
}
