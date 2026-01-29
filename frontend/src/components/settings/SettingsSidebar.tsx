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

    return items;
  };

  const isActive = (url: string): boolean => {
    if (!url) return false;
    const currentPath = window.location.pathname;
    const currentHash = window.location.hash;
    const [urlPath, urlHash] = url.split('#');

    // Check path match
    if (urlPath !== currentPath) return false;

    // If URL has hash, check hash match
    if (urlHash) {
      return currentHash === '#' + urlHash;
    }

    // If no hash in URL, match if no hash in current location or hash matches any section
    return true;
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
