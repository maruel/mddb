// Sidebar navigation component containing workspace selection and page tree.

import { createSignal, For, Show, createMemo } from 'solid-js';
import SidebarNode from './SidebarNode';
import { useI18n } from '../i18n';
import { useAuth } from '../contexts';
import { useWorkspace } from '../contexts';
import type { NodeResponse, WSMembershipResponse } from '@sdk/types.gen';
import { WSRoleAdmin } from '@sdk/types.gen';
import styles from './Sidebar.module.css';

interface OrgWithWorkspaces {
  orgId: string;
  orgName: string;
  workspaces: WSMembershipResponse[];
}

interface SidebarProps {
  isOpen: boolean;
  loading: boolean;
  nodes: NodeResponse[];
  selectedNodeId: string | null;
  ancestorIds: string[];
  onCreatePage: () => void;
  onCreateTable: () => void;
  onCreateChildPage: (parentId: string) => void;
  onCreateChildTable: (parentId: string) => void;
  onSelectNode: (node: NodeResponse) => void;
  onCloseMobileSidebar: () => void;
  onFetchChildren?: (nodeId: string) => Promise<void>;
  onDeleteNode?: (nodeId: string) => void;
  onShowHistory?: (nodeId: string) => void;
  onOpenSettings: () => void;
  onCreateWorkspace: () => void;
  onImportFromNotion: () => void;
}

export default function Sidebar(props: SidebarProps) {
  const { t } = useI18n();
  const { user, wsApi, refreshUser } = useAuth();
  const { switchWorkspace } = useWorkspace();
  const [showOtherWorkspaces, setShowOtherWorkspaces] = createSignal(false);
  const [isEditingName, setIsEditingName] = createSignal(false);
  const [editNameValue, setEditNameValue] = createSignal('');
  const [isSaving, setIsSaving] = createSignal(false);

  const currentWsId = () => user()?.workspace_id || '';
  const currentWsName = () => {
    const u = user();
    if (!u) return t('app.workspace');
    const current = u.workspaces?.find((ws) => ws.workspace_id === u.workspace_id);
    return current?.workspace_name || t('app.workspace');
  };
  const isAdmin = () => user()?.workspace_role === WSRoleAdmin;

  const startEditingName = () => {
    if (!isAdmin()) return;
    setEditNameValue(currentWsName());
    setIsEditingName(true);
  };

  const cancelEditingName = () => {
    setIsEditingName(false);
    setEditNameValue('');
  };

  const saveWorkspaceName = async () => {
    const ws = wsApi();
    const newName = editNameValue().trim();
    if (!ws || !newName || newName === currentWsName()) {
      cancelEditingName();
      return;
    }

    try {
      setIsSaving(true);
      await ws.workspaces.updateWorkspace({ name: newName });
      await refreshUser();
      setIsEditingName(false);
    } catch (err) {
      console.error('Failed to update workspace name:', err);
      // Keep edit mode open on error so user can retry or cancel
    } finally {
      setIsSaving(false);
    }
  };

  const handleNameKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      saveWorkspaceName();
    } else if (e.key === 'Escape') {
      e.preventDefault();
      cancelEditingName();
    }
  };

  // Group workspaces by organization
  const groupedWorkspaces = createMemo((): OrgWithWorkspaces[] => {
    const u = user();
    if (!u) return [];

    const orgMap = new Map<string, OrgWithWorkspaces>();
    const organizations = u.organizations || [];
    const workspaces = u.workspaces || [];

    for (const org of organizations) {
      orgMap.set(org.organization_id, {
        orgId: org.organization_id,
        orgName: org.organization_name || org.organization_id,
        workspaces: [],
      });
    }

    for (const ws of workspaces) {
      const org = orgMap.get(ws.organization_id);
      if (org) {
        org.workspaces.push(ws);
      }
    }

    return Array.from(orgMap.values()).filter((o) => o.workspaces.length > 0);
  });

  // Other workspaces (excluding current)
  const otherWorkspaces = createMemo(() => {
    const wsId = currentWsId();
    return groupedWorkspaces().flatMap((org) => org.workspaces.filter((ws) => ws.workspace_id !== wsId));
  });

  const hasMultipleOrgs = createMemo(() => groupedWorkspaces().length > 1);

  const handleSwitchWorkspace = (wsId: string) => {
    if (wsId !== currentWsId()) {
      switchWorkspace(wsId);
    }
  };

  return (
    <aside class={`${styles.sidebar} ${props.isOpen ? styles.open : ''}`}>
      {/* Current workspace header */}
      <div class={styles.workspaceHeader}>
        <Show
          when={isEditingName()}
          fallback={
            <span
              class={`${styles.workspaceName} ${isAdmin() ? styles.editable : ''}`}
              onClick={startEditingName}
              title={isAdmin() ? t('settings.clickToEditName') || 'Click to edit name' : undefined}
            >
              {currentWsName()}
            </span>
          }
        >
          <input
            ref={(el) => setTimeout(() => el.focus(), 0)}
            type="text"
            class={styles.workspaceNameInput}
            value={editNameValue()}
            onInput={(e) => setEditNameValue(e.target.value)}
            onKeyDown={handleNameKeyDown}
            onBlur={saveWorkspaceName}
            disabled={isSaving()}
          />
        </Show>
        <button
          class={styles.settingsButton}
          onClick={() => props.onOpenSettings()}
          title={t('app.settings') || 'Settings'}
          data-testid="workspace-settings-button"
        >
          ⚙
        </button>
        <button
          class={styles.collapseButton}
          onClick={() => props.onCloseMobileSidebar()}
          title={t('app.collapseSidebar') || 'Collapse sidebar'}
        >
          «
        </button>
      </div>

      <Show when={props.loading && props.nodes.length === 0}>
        <p class={styles.loading}>{t('common.loading')}</p>
      </Show>

      {/* Current workspace pages */}
      <ul class={styles.pageList}>
        <For each={props.nodes}>
          {(node) => (
            <SidebarNode
              node={node}
              selectedId={props.selectedNodeId}
              ancestorIds={props.ancestorIds}
              onSelect={props.onSelectNode}
              onCreateChildPage={props.onCreateChildPage}
              onCreateChildTable={props.onCreateChildTable}
              onFetchChildren={props.onFetchChildren}
              onDeleteNode={props.onDeleteNode}
              onShowHistory={props.onShowHistory}
              depth={0}
            />
          )}
        </For>
      </ul>

      {/* Other workspaces section */}
      <div class={styles.otherWorkspaces}>
        <Show when={otherWorkspaces().length > 0}>
          <button class={styles.otherWorkspacesToggle} onClick={() => setShowOtherWorkspaces(!showOtherWorkspaces())}>
            <span class={`${styles.toggleChevron} ${showOtherWorkspaces() ? styles.expanded : ''}`}>▶</span>
            {t('app.otherWorkspaces') || 'Other workspaces'}
          </button>

          <Show when={showOtherWorkspaces()}>
            <div class={styles.otherWorkspacesList}>
              <For each={groupedWorkspaces()}>
                {(org) => (
                  <>
                    <Show when={hasMultipleOrgs()}>
                      <div class={styles.orgLabel}>{org.orgName}</div>
                    </Show>
                    <For each={org.workspaces.filter((ws) => ws.workspace_id !== currentWsId())}>
                      {(ws) => (
                        <button
                          class={styles.otherWorkspaceItem}
                          onClick={() => handleSwitchWorkspace(ws.workspace_id)}
                        >
                          {ws.workspace_name || ws.workspace_id}
                        </button>
                      )}
                    </For>
                  </>
                )}
              </For>
            </div>
          </Show>
        </Show>

        {/* Actions */}
        <div class={styles.sidebarActions}>
          <button
            class={styles.actionButton}
            onClick={() => props.onCreateWorkspace()}
            data-testid="create-workspace-button"
          >
            <span class={styles.actionIcon}>+</span>
            {t('createWorkspace.title') || 'Create workspace'}
          </button>
          <button
            class={styles.actionButton}
            onClick={() => props.onImportFromNotion()}
            data-testid="import-notion-button"
          >
            <span class={styles.actionIcon}>↓</span>
            {t('notionImport.title') || 'Import from Notion'}
          </button>
        </div>
      </div>
    </aside>
  );
}
