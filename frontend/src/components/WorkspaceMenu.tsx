// Dropdown menu for switching workspaces and accessing workspace settings.

import { createSignal, For, onMount, onCleanup, Show, createMemo } from 'solid-js';
import { useI18n } from '../i18n';
import type { WSMembershipResponse, OrgMembershipResponse } from '@sdk/types.gen';
import styles from './WorkspaceMenu.module.css';

interface WorkspaceMenuProps {
  workspaces: WSMembershipResponse[];
  organizations: OrgMembershipResponse[];
  currentWsId: string;
  onSwitchWorkspace: (wsId: string) => void;
  onOpenSettings: () => void;
  onCreateWorkspace: () => void;
}

interface OrgWithWorkspaces {
  orgId: string;
  orgName: string;
  workspaces: WSMembershipResponse[];
}

export default function WorkspaceMenu(props: WorkspaceMenuProps) {
  const { t } = useI18n();
  const [isOpen, setIsOpen] = createSignal(false);
  let menuRef: HTMLDivElement | undefined;

  // Group workspaces by organization
  const groupedWorkspaces = createMemo((): OrgWithWorkspaces[] => {
    const orgMap = new Map<string, OrgWithWorkspaces>();

    // Initialize with org info
    for (const org of props.organizations) {
      orgMap.set(org.organization_id, {
        orgId: org.organization_id,
        orgName: org.organization_name || org.organization_id,
        workspaces: [],
      });
    }

    // Add workspaces to their orgs
    for (const ws of props.workspaces) {
      const org = orgMap.get(ws.organization_id);
      if (org) {
        org.workspaces.push(ws);
      }
    }

    // Return as array, filtering out orgs with no workspaces
    return Array.from(orgMap.values()).filter((o) => o.workspaces.length > 0);
  });

  const currentWsName = () => {
    const current = props.workspaces.find((ws) => ws.workspace_id === props.currentWsId);
    return current?.workspace_name || t('app.workspace');
  };

  const handleClickOutside = (e: MouseEvent) => {
    if (menuRef && !menuRef.contains(e.target as Node)) {
      setIsOpen(false);
    }
  };

  onMount(() => {
    document.addEventListener('mousedown', handleClickOutside);
    onCleanup(() => {
      document.removeEventListener('mousedown', handleClickOutside);
    });
  });

  const handleSwitchWorkspace = (wsId: string) => {
    setIsOpen(false);
    if (wsId !== props.currentWsId) {
      props.onSwitchWorkspace(wsId);
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

  // Check if we have multiple orgs (to decide whether to show org headers)
  const hasMultipleOrgs = createMemo(() => groupedWorkspaces().length > 1);

  return (
    <div class={styles.wsMenu} ref={menuRef}>
      <button class={styles.wsButton} onClick={() => setIsOpen(!isOpen())} title={currentWsName()}>
        <span class={styles.wsName}>{currentWsName()}</span>
        <span class={styles.chevron}>{isOpen() ? '▲' : '▼'}</span>
      </button>

      <Show when={isOpen()}>
        <div class={styles.dropdown}>
          <div class={styles.wsList}>
            <For each={groupedWorkspaces()}>
              {(org) => (
                <>
                  <Show when={hasMultipleOrgs()}>
                    <div class={styles.orgHeader}>{org.orgName}</div>
                  </Show>
                  <For each={org.workspaces}>
                    {(ws) => (
                      <button
                        class={`${styles.wsItem} ${ws.workspace_id === props.currentWsId ? styles.active : ''}`}
                        onClick={() => handleSwitchWorkspace(ws.workspace_id)}
                      >
                        <span class={styles.wsItemName}>{ws.workspace_name || ws.workspace_id}</span>
                        <Show when={ws.workspace_id === props.currentWsId}>
                          <span class={styles.checkmark}>✓</span>
                        </Show>
                      </button>
                    )}
                  </For>
                </>
              )}
            </For>
          </div>
          <div class={styles.divider} />
          <button class={styles.menuItem} onClick={handleOpenSettings}>
            <span class={styles.icon}>⚙</span>
            {t('app.settings')}
          </button>
          <button class={styles.menuItem} onClick={handleCreateWorkspace}>
            <span class={styles.plusIcon}>+</span>
            {t('createWorkspace.title')}
          </button>
        </div>
      </Show>
    </div>
  );
}
