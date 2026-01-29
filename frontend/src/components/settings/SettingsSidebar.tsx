// Settings sidebar navigation with expandable workspace and organization items.

import { For } from 'solid-js';
import { useAuth } from '../../contexts';
import { useI18n } from '../../i18n';
import { settingsUrl, type UnifiedSettingsMatch } from '../../utils/urls';
import { OrgRoleAdmin, OrgRoleOwner, WSRoleAdmin } from '@sdk/types.gen';
import SettingsNavItem from './SettingsNavItem';
import styles from './SettingsSidebar.module.css';

interface SettingsSidebarProps {
  isOpen: boolean;
  currentRoute: UnifiedSettingsMatch;
  onNavigate: (url: string) => void;
}

export interface NavItem {
  id: string;
  label: string;
  url: string;
  children?: NavItem[];
}

export default function SettingsSidebar(props: SettingsSidebarProps) {
  const { t } = useI18n();
  const { user } = useAuth();

  // Build navigation tree
  const navItems = (): NavItem[] => {
    const items: NavItem[] = [];
    const u = user();
    if (!u) return items;

    // User profile
    items.push({
      id: 'user',
      label: t('settings.personal'),
      url: settingsUrl('user'),
    });

    // Workspaces
    const workspaces = u.workspaces || [];
    if (workspaces.length > 0) {
      const wsChildren: NavItem[] = workspaces.map((ws) => {
        const wsId = ws.workspace_id;
        const wsName = ws.workspace_name || wsId;
        const isAdmin = ws.role === WSRoleAdmin;
        const children: NavItem[] = [
          {
            id: `ws-${wsId}-members`,
            label: t('settings.members'),
            url: settingsUrl('workspace', wsId, wsName) + '#members',
          },
          {
            id: `ws-${wsId}-settings`,
            label: t('settings.settings'),
            url: settingsUrl('workspace', wsId, wsName) + '#settings',
          },
        ];
        if (isAdmin) {
          children.push({
            id: `ws-${wsId}-sync`,
            label: t('settings.gitSync'),
            url: settingsUrl('workspace', wsId, wsName) + '#sync',
          });
        }
        return {
          id: `ws-${wsId}`,
          label: wsName,
          url: settingsUrl('workspace', wsId, wsName),
          children,
        };
      });

      items.push({
        id: 'workspaces',
        label: t('settings.workspaces'),
        url: '',
        children: wsChildren,
      });
    }

    // Organizations (only admin/owner)
    const orgs = (u.organizations || []).filter((o) => o.role === OrgRoleAdmin || o.role === OrgRoleOwner);
    if (orgs.length > 0) {
      const orgChildren: NavItem[] = orgs.map((org) => {
        const orgId = org.organization_id;
        const orgName = org.organization_name || orgId;
        return {
          id: `org-${orgId}`,
          label: orgName,
          url: settingsUrl('org', orgId, orgName),
          children: [
            {
              id: `org-${orgId}-members`,
              label: t('settings.members'),
              url: settingsUrl('org', orgId, orgName) + '#members',
            },
            {
              id: `org-${orgId}-settings`,
              label: t('settings.settings'),
              url: settingsUrl('org', orgId, orgName) + '#settings',
            },
          ],
        };
      });

      items.push({
        id: 'organizations',
        label: t('settings.organizations'),
        url: '',
        children: orgChildren,
      });
    }

    // Server settings (only for global admins)
    if (u.is_global_admin) {
      items.push({
        id: 'server',
        label: t('server.serverSettings'),
        url: settingsUrl('server'),
      });
    }

    return items;
  };

  // Note: route is passed as parameter (not accessed from closure) so SolidJS
  // tracks the dependency in the component that calls this function.
  const isActive = (url: string, route: UnifiedSettingsMatch): boolean => {
    if (!url) return false;

    // Match based on route type and URL
    if (url === '/settings/user' && route.type === 'profile') return true;
    if (url === '/settings/server' && route.type === 'server') return true;

    // For workspace/org URLs, check if the ID matches
    if (route.type === 'workspace' && route.id) {
      const wsMatch = url.match(/^\/settings\/workspace\/([^+/]+)/);
      if (wsMatch && wsMatch[1] === route.id) {
        // Check section/hash match
        const [, urlHash] = url.split('#');
        if (urlHash) {
          return route.section === urlHash;
        }
        return !route.section; // Active if no hash in URL and no section in route
      }
    }

    if (route.type === 'org' && route.id) {
      const orgMatch = url.match(/^\/settings\/org\/([^+/]+)/);
      if (orgMatch && orgMatch[1] === route.id) {
        // Check section/hash match
        const [, urlHash] = url.split('#');
        if (urlHash) {
          return route.section === urlHash;
        }
        return !route.section;
      }
    }

    return false;
  };

  return (
    <aside class={`${styles.sidebar} ${props.isOpen ? styles.mobileOpen : ''}`}>
      <nav class={styles.nav}>
        <For each={navItems()}>
          {(item) => (
            <SettingsNavItem
              item={item}
              depth={0}
              isActive={isActive}
              onNavigate={props.onNavigate}
              currentRoute={props.currentRoute}
            />
          )}
        </For>
      </nav>
    </aside>
  );
}
