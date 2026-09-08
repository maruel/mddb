// Dropdown menu for switching workspaces and accessing workspace settings.

import { createSignal, For, Show, createMemo } from 'solid-js';
import { useI18n } from '../i18n';
import { useAuth } from '../contexts';
import { useWorkspace } from '../contexts';
import { useClickOutside } from '../composables/useClickOutside';
import { Button, Menu, MenuItem } from './shared';
import type { WSMembershipResponse } from '@sdk/types.gen';
import styles from './WorkspaceMenu.module.css';

import ExpandLessIcon from '@material-symbols/svg-400/outlined/expand_less.svg?solid';
import ExpandMoreIcon from '@material-symbols/svg-400/outlined/expand_more.svg?solid';
import CheckIcon from '@material-symbols/svg-400/outlined/check.svg?solid';
import SettingsIcon from '@material-symbols/svg-400/outlined/settings.svg?solid';
import AddIcon from '@material-symbols/svg-400/outlined/add.svg?solid';
import DownloadIcon from '@material-symbols/svg-400/outlined/download.svg?solid';

interface WorkspaceMenuProps {
  onOpenSettings: () => void;
  onCreateWorkspace: () => void;
  onImportFromNotion: () => void;
  onSwitchWorkspace?: () => void;
}

interface OrgWithWorkspaces {
  orgId: string;
  orgName: string;
  workspaces: WSMembershipResponse[];
}

export default function WorkspaceMenu(props: WorkspaceMenuProps) {
  const { t } = useI18n();
  const { user } = useAuth();
  const { switchWorkspace } = useWorkspace();
  const [isOpen, setIsOpen] = createSignal(false);
  let menuRef: HTMLDivElement | undefined;
  let triggerRef: HTMLButtonElement | undefined;

  // Group workspaces by organization
  const groupedWorkspaces = createMemo((): OrgWithWorkspaces[] => {
    const u = user();
    if (!u) return [];

    const orgMap = new Map<string, OrgWithWorkspaces>();
    const organizations = u.organizations || [];
    const workspaces = u.workspaces || [];

    // Initialize with org info
    for (const org of organizations) {
      orgMap.set(org.organization_id, {
        orgId: org.organization_id,
        orgName: org.organization_name || org.organization_id,
        workspaces: [],
      });
    }

    // Add workspaces to their orgs
    for (const ws of workspaces) {
      const org = orgMap.get(ws.organization_id);
      if (org) {
        org.workspaces.push(ws);
      }
    }

    // Return as array, filtering out orgs with no workspaces
    return Array.from(orgMap.values()).filter((o) => o.workspaces.length > 0);
  });

  const currentWsId = () => user()?.workspace_id || '';

  const currentWsName = () => {
    const u = user();
    if (!u) return t('app.workspace');
    const current = u.workspaces?.find((ws) => ws.workspace_id === u.workspace_id);
    return current?.workspace_name || t('app.workspace');
  };

  useClickOutside(
    () => menuRef,
    () => setIsOpen(false)
  );

  const handleSwitchWorkspace = async (wsId: string) => {
    setIsOpen(false);
    props.onSwitchWorkspace?.();
    if (wsId !== currentWsId()) {
      await switchWorkspace(wsId);
    }
  };

  const handleOpenSettings = () => {
    setIsOpen(false);
    props.onOpenSettings();
  };

  const handleCreateWorkspace = () => {
    setIsOpen(false);
    props.onCreateWorkspace();
  };

  const handleImportFromNotion = () => {
    setIsOpen(false);
    props.onImportFromNotion();
  };

  // Check if we have multiple orgs (to decide whether to show org headers)
  const hasMultipleOrgs = createMemo(() => groupedWorkspaces().length > 1);

  return (
    <div class={styles.wsMenu} ref={(el) => (menuRef = el)}>
      <Button
        ref={(el) => (triggerRef = el)}
        variant="unstyled"
        class={styles.wsButton}
        onClick={() => setIsOpen(!isOpen())}
        title={currentWsName()}
        aria-label={t('app.switchWorkspace') || 'Switch workspace'}
        aria-expanded={isOpen()}
        aria-haspopup="menu"
      >
        <span class={styles.wsName}>{currentWsName()}</span>
        <span class={styles.chevron} aria-hidden="true">
          {isOpen() ? <ExpandLessIcon /> : <ExpandMoreIcon />}
        </span>
      </Button>

      <Show when={isOpen()}>
        <Menu
          ariaLabel={t('app.switchWorkspace') || 'Switch workspace'}
          class={styles.dropdown}
          onClose={() => setIsOpen(false)}
          trigger={() => triggerRef}
        >
          <div class={styles.wsList}>
            <For each={groupedWorkspaces()}>
              {(org) => (
                <>
                  <Show when={hasMultipleOrgs()}>
                    <div class={styles.orgHeader}>{org.orgName}</div>
                  </Show>
                  <For each={org.workspaces}>
                    {(ws) => (
                      <MenuItem
                        class={`${styles.wsItem} ${ws.workspace_id === currentWsId() ? styles.active : ''}`}
                        onClick={() => handleSwitchWorkspace(ws.workspace_id)}
                      >
                        <span class={styles.wsItemName}>{ws.workspace_name || ws.workspace_id}</span>
                        <Show when={ws.workspace_id === currentWsId()}>
                          <span class={styles.checkmark} aria-hidden="true">
                            <CheckIcon />
                          </span>
                        </Show>
                      </MenuItem>
                    )}
                  </For>
                </>
              )}
            </For>
          </div>
          <div class={styles.divider} />
          <MenuItem class={styles.menuItem} onClick={handleOpenSettings}>
            <span class={styles.icon} aria-hidden="true">
              <SettingsIcon />
            </span>
            {t('app.settings')}
          </MenuItem>
          <MenuItem class={styles.menuItem} onClick={handleCreateWorkspace}>
            <span class={styles.plusIcon} aria-hidden="true">
              <AddIcon />
            </span>
            {t('createWorkspace.title')}
          </MenuItem>
          <MenuItem class={styles.menuItem} onClick={handleImportFromNotion}>
            <span class={styles.icon} aria-hidden="true">
              <DownloadIcon />
            </span>
            {t('notionImport.title')}
          </MenuItem>
        </Menu>
      </Show>
    </div>
  );
}
