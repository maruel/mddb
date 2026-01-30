// Settings layout with sidebar navigation and content outlet.

import { createSignal, Show, onMount, onCleanup, type JSX } from 'solid-js';
import { useNavigate, useLocation } from '@solidjs/router';
import { useAuth } from '../contexts';
import { useI18n } from '../i18n';
import { workspaceUrl } from '../utils/urls';
import SettingsSidebar from '../components/settings/SettingsSidebar';
import styles from './SettingsSection.module.css';

interface SettingsLayoutProps {
  children?: JSX.Element;
}

export default function SettingsLayout(props: SettingsLayoutProps) {
  const { t } = useI18n();
  const { user } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
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

  const handleBack = () => {
    navigate(-1);
  };

  const navigateToWorkspace = () => {
    const wsId = user()?.workspace_id;
    const wsName = user()?.workspace_name;
    if (wsId) {
      navigate(workspaceUrl(wsId, wsName));
    }
  };

  const handleNavigate = (url: string) => {
    setShowMobileSidebar(false);
    navigate(url);
  };

  // Derive current route info from location for sidebar highlighting
  const currentRoute = () => {
    const path = location.pathname;
    const hash = location.hash?.slice(1) || undefined;

    if (path === '/settings/user' || path === '/settings/user/') {
      return { type: 'profile' as const };
    }
    if (path === '/settings/server' || path === '/settings/server/') {
      return { type: 'server' as const };
    }

    const wsMatch = path.match(/^\/settings\/workspace\/([^+/]+)/);
    if (wsMatch) {
      return { type: 'workspace' as const, id: wsMatch[1], section: hash };
    }

    const orgMatch = path.match(/^\/settings\/org\/([^+/]+)/);
    if (orgMatch) {
      return { type: 'org' as const, id: orgMatch[1], section: hash };
    }

    return { type: 'profile' as const };
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
            &#9776;
          </button>
          <button onClick={handleBack} class={styles.backButton}>
            &larr; {t('common.back')}
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

        <SettingsSidebar isOpen={showMobileSidebar()} currentRoute={currentRoute()} onNavigate={handleNavigate} />

        <main class={styles.content}>{props.children}</main>
      </div>
    </div>
  );
}
