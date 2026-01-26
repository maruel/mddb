// Dropdown menu for switching workspaces and accessing workspace settings.

import { createSignal, For, onMount, onCleanup, Show, createMemo } from 'solid-js';
import { useI18n } from '../i18n';
import type { WSMembershipResponse } from '../types.gen';
import styles from './WorkspaceMenu.module.css';

interface WorkspaceMenuProps {
  workspaces: WSMembershipResponse[];
  currentOrgId: string;
  currentWsId: string;
  onSwitchWorkspace: (wsId: string) => void;
  onOpenSettings: () => void;
  onCreateWorkspace: () => void;
}

export default function WorkspaceMenu(props: WorkspaceMenuProps) {
  const { t } = useI18n();
  const [isOpen, setIsOpen] = createSignal(false);
  let menuRef: HTMLDivElement | undefined;

  // Filter workspaces by current organization
  const orgWorkspaces = createMemo(() => props.workspaces.filter((ws) => ws.organization_id === props.currentOrgId));

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

  return (
    <div class={styles.wsMenu} ref={menuRef}>
      <button class={styles.wsButton} onClick={() => setIsOpen(!isOpen())} title={currentWsName()}>
        <span class={styles.wsName}>{currentWsName()}</span>
        <span class={styles.chevron}>{isOpen() ? '▲' : '▼'}</span>
      </button>

      <Show when={isOpen()}>
        <div class={styles.dropdown}>
          <div class={styles.wsList}>
            <For each={orgWorkspaces()}>
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
