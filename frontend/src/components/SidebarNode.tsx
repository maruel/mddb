// Recursive component for rendering navigation tree nodes in the sidebar.

import { createSignal, createEffect, For, Show, on, untrack } from 'solid-js';
import { useI18n } from '../i18n';
import type { NodeResponse } from '@sdk/types.gen';
import { ContextMenu, type ContextMenuAction } from './shared';
import styles from './SidebarNode.module.css';

import DescriptionIcon from '@material-symbols/svg-400/outlined/description.svg?solid';
import TableChartIcon from '@material-symbols/svg-400/outlined/table_chart.svg?solid';
import ChevronRightIcon from '@material-symbols/svg-400/outlined/chevron_right.svg?solid';
import DeleteIcon from '@material-symbols/svg-400/outlined/delete.svg?solid';
import HistoryIcon from '@material-symbols/svg-400/outlined/history.svg?solid';
import { IconDisplay } from './editor/IconPicker';

// Module-level drag state (imperative, not reactive)
let dragState: { nodeId: string; element: HTMLElement } | null = null;

interface SidebarNodeProps {
  node: NodeResponse;
  selectedId: string | null;
  ancestorIds: string[];
  focusedNodeId: string | null;
  movingNodeId: string | null;
  onSelect: (node: NodeResponse) => void;
  onFocusNode: (nodeId: string) => void;
  onStartMove: (nodeId: string) => void;
  onCreateChildPage: (parentId: string) => void;
  onCreateChildTable: (parentId: string) => void;
  onFetchChildren?: (nodeId: string) => Promise<void>;
  onDeleteNode?: (nodeId: string) => void;
  onShowHistory?: (nodeId: string) => void;
  onMoveNode?: (nodeId: string, newParentId: string) => Promise<void> | void;
  depth: number;
  isFirstTreeItem: boolean;
}

export default function SidebarNode(props: SidebarNodeProps) {
  const { t } = useI18n();
  // Start expanded if depth 0 OR if already an ancestor at mount time
  const initialExpanded = untrack(() => props.depth === 0 || props.ancestorIds.includes(props.node.id));
  const [isExpanded, setIsExpanded] = createSignal(initialExpanded);
  const [showContextMenu, setShowContextMenu] = createSignal(false);
  const [contextMenuPos, setContextMenuPos] = createSignal({ x: 0, y: 0 });
  const [isLoadingChildren, setIsLoadingChildren] = createSignal(false);

  // Children come directly from the store via props.node.children
  const children = () => props.node.children ?? [];

  // Check if this node might have children
  const mightHaveChildren = () => {
    if (props.node.children && props.node.children.length > 0) return true;
    return props.node.has_children === true;
  };

  // Auto-expand when the selected node is a direct child of this node
  createEffect(() => {
    const selectedId = props.selectedId;
    const nodeChildren = children();
    if (selectedId && nodeChildren.some((child) => child.id === selectedId)) {
      setIsExpanded(true);
    }
  });

  // Auto-expand for direct URL navigation when ancestorIds becomes available
  // Track whether we've auto-expanded to prevent re-expanding after user collapses
  const [hasAutoExpanded, setHasAutoExpanded] = createSignal(false);
  createEffect(
    on(
      () => props.ancestorIds.length,
      (length, prevLength) => {
        const wasEmpty = prevLength === undefined || prevLength === 0;
        if (wasEmpty && length > 0 && !hasAutoExpanded() && props.ancestorIds.includes(props.node.id)) {
          setHasAutoExpanded(true);
          setIsExpanded(true);
        }
      }
    )
  );

  // Fetch children when expanded and not yet loaded
  createEffect(() => {
    if (isExpanded() && mightHaveChildren() && !props.node.children && props.onFetchChildren) {
      setIsLoadingChildren(true);
      props.onFetchChildren(props.node.id).finally(() => setIsLoadingChildren(false));
    }
  });

  const toggleExpand = () => {
    const focusedItem = treeItems().find((item) => item.dataset.nodeId === props.focusedNodeId);
    if (isExpanded() && focusedItem && focusedItem !== treeItemRef && liRef.contains(focusedItem)) {
      focusNode(props.node.id);
    }
    setIsExpanded(!isExpanded());
  };

  const handleContextMenu = (e: MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setContextMenuPos({ x: e.clientX, y: e.clientY });
    setShowContextMenu(true);
  };

  // Drag-and-drop state
  let liRef!: HTMLLIElement;
  let treeItemRef: HTMLButtonElement | undefined;
  const [isDragging, setIsDragging] = createSignal(false);
  const [isDropTarget, setIsDropTarget] = createSignal(false);
  let expandTimer: ReturnType<typeof setTimeout> | undefined;

  const handleDragStart = (e: DragEvent) => {
    if (!e.dataTransfer) return;
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', props.node.id);
    dragState = { nodeId: props.node.id, element: liRef };
    setIsDragging(true);
  };

  const handleDragEnd = () => {
    dragState = null;
    setIsDragging(false);
    setIsDropTarget(false);
    clearTimeout(expandTimer);
  };

  const handleDragOver = (e: DragEvent) => {
    if (!dragState) return;
    // Reject: dropping on self
    if (dragState.nodeId === props.node.id) return;
    // Reject: dropping on a visible descendant (DOM containment check)
    if (dragState.element.contains(e.currentTarget as Node)) return;

    e.preventDefault();
    e.stopPropagation();
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
    setIsDropTarget(true);

    // Auto-expand collapsed nodes with children after 600ms hover
    if (!expandTimer && mightHaveChildren() && !isExpanded()) {
      expandTimer = setTimeout(() => {
        setIsExpanded(true);
        if (!props.node.children && props.onFetchChildren) {
          setIsLoadingChildren(true);
          props.onFetchChildren(props.node.id).finally(() => setIsLoadingChildren(false));
        }
      }, 600);
    }
  };

  const handleDragLeave = (e: DragEvent) => {
    // Only clear if we're actually leaving this element, not entering a child
    const related = e.relatedTarget as Node | null;
    if (related && (e.currentTarget as Node).contains(related)) return;
    setIsDropTarget(false);
    clearTimeout(expandTimer);
    expandTimer = undefined;
  };

  const handleDrop = (e: DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDropTarget(false);
    clearTimeout(expandTimer);
    expandTimer = undefined;
    if (dragState && dragState.nodeId !== props.node.id) {
      void props.onMoveNode?.(dragState.nodeId, props.node.id);
    }
  };

  const treeItems = () =>
    Array.from(liRef.closest('[role="tree"]')?.querySelectorAll<HTMLButtonElement>('[role="treeitem"]') ?? []);

  const focusNode = (nodeId: string) => {
    const target = treeItems().find((item) => item.dataset.nodeId === nodeId);
    if (!target) return;
    props.onFocusNode(nodeId);
    target.focus();
  };

  const focusAtOffset = (offset: number) => {
    const items = treeItems();
    const index = items.indexOf(treeItemRef as HTMLButtonElement);
    const target = items[index + offset];
    if (!target) return;
    const nodeId = target.dataset.nodeId;
    if (nodeId) {
      props.onFocusNode(nodeId);
      target.focus();
    }
  };

  const canMoveHere = () => {
    const sourceId = props.movingNodeId;
    if (!sourceId || sourceId === props.node.id) return false;
    const sourceItem = treeItems().find((item) => item.dataset.nodeId === sourceId);
    return !sourceItem?.closest('li')?.contains(liRef);
  };

  const focusBeforeDelete = () => {
    if (props.focusedNodeId !== props.node.id) return;
    let target: HTMLButtonElement | undefined;
    if (props.node.parent_id && props.node.parent_id !== '0') {
      target = treeItems().find((item) => item.dataset.nodeId === props.node.parent_id);
    } else {
      const items = treeItems();
      const index = items.indexOf(treeItemRef as HTMLButtonElement);
      target = items[index + 1] ?? items[index - 1];
    }
    const targetId = target?.dataset.nodeId;
    if (target && targetId) {
      props.onFocusNode(targetId);
      queueMicrotask(() => {
        if (target?.isConnected) target.focus();
      });
    }
  };

  const requestDelete = () => {
    focusBeforeDelete();
    props.onDeleteNode?.(props.node.id);
  };

  const openContextMenu = () => {
    const rect = treeItemRef?.getBoundingClientRect();
    setContextMenuPos({ x: rect?.left ?? 0, y: rect?.bottom ?? 0 });
    setShowContextMenu(true);
  };

  const handleTreeKeyDown = (event: KeyboardEvent) => {
    switch (event.key) {
      case 'ArrowDown':
        event.preventDefault();
        focusAtOffset(1);
        break;
      case 'ArrowUp':
        event.preventDefault();
        focusAtOffset(-1);
        break;
      case 'Home': {
        event.preventDefault();
        const first = treeItems()[0];
        const nodeId = first?.dataset.nodeId;
        if (first && nodeId) {
          props.onFocusNode(nodeId);
          first.focus();
        }
        break;
      }
      case 'End': {
        event.preventDefault();
        const items = treeItems();
        const last = items[items.length - 1];
        const nodeId = last?.dataset.nodeId;
        if (last && nodeId) {
          props.onFocusNode(nodeId);
          last.focus();
        }
        break;
      }
      case 'ArrowRight':
        if (!mightHaveChildren()) break;
        event.preventDefault();
        if (!isExpanded()) {
          setIsExpanded(true);
        } else {
          const firstChildId = children()[0]?.id;
          if (firstChildId) {
            focusNode(firstChildId);
          }
        }
        break;
      case 'ArrowLeft':
        event.preventDefault();
        if (mightHaveChildren() && isExpanded()) {
          setIsExpanded(false);
        } else if (props.node.parent_id && props.node.parent_id !== '0') {
          focusNode(props.node.parent_id);
        }
        break;
      case 'Enter':
      case ' ':
        event.preventDefault();
        props.onSelect(props.node);
        break;
      case 'ContextMenu':
        event.preventDefault();
        openContextMenu();
        break;
      case 'F10':
        if (event.shiftKey) {
          event.preventDefault();
          openContextMenu();
        }
        break;
      case 'v':
      case 'V':
        if (event.ctrlKey && canMoveHere() && props.movingNodeId) {
          event.preventDefault();
          void props.onMoveNode?.(props.movingNodeId, props.node.id);
        }
        break;
    }
  };

  const contextActions = (): ContextMenuAction[] => {
    const actions: ContextMenuAction[] = [
      {
        id: 'create-subpage',
        label: t('app.createSubPage') || 'Create sub-page',
        icon: <DescriptionIcon />,
      },
      {
        id: 'create-subtable',
        label: t('app.createSubTable') || 'Create sub-table',
        icon: <TableChartIcon />,
      },
      {
        id: 'history',
        label: t('editor.history') || 'History',
        icon: <HistoryIcon />,
        separator: true,
      },
      {
        id: 'move-node',
        label: t('app.moveNode'),
        separator: true,
      },
      {
        id: 'move-root',
        label: t('app.moveToRoot'),
      },
      {
        id: 'delete-node',
        label: t('common.delete'),
        danger: true,
        separator: true,
      },
    ];

    if (canMoveHere()) {
      actions.push({
        id: 'move-here',
        label: t('app.moveNodeHere'),
        separator: true,
      });
    }

    return actions;
  };

  return (
    <li
      ref={(el) => (liRef = el)}
      class={styles.sidebarNodeWrapper}
      role="none"
      data-testid={`sidebar-node-${props.node.id}`}
    >
      <div
        class={styles.pageItem}
        classList={{
          [`${styles.active}`]: props.selectedId === props.node.id,
          [`${styles.dragging}`]: isDragging(),
          [`${styles.dropTarget}`]: isDropTarget(),
        }}
        style={{ 'padding-left': `${props.depth * 12 + 8}px` }}
        draggable="true"
        onClick={() => props.onSelect(props.node)}
        onContextMenu={handleContextMenu}
        onDragStart={handleDragStart}
        onDragEnd={handleDragEnd}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
      >
        <Show
          when={mightHaveChildren()}
          fallback={
            <span class={styles.iconSlot} aria-hidden="true">
              <span class={styles.nodeIcon}>
                {props.node.icon ? (
                  <IconDisplay icon={props.node.icon} class={styles.sidebarIcon} />
                ) : props.node.has_table && !props.node.has_page ? (
                  <TableChartIcon />
                ) : (
                  <DescriptionIcon />
                )}
              </span>
            </span>
          }
        >
          <span
            class={styles.expandButton}
            data-testid={`expand-icon-${props.node.id}`}
            aria-hidden="true"
            title={
              isExpanded()
                ? t('app.collapseNode', { title: props.node.title })
                : t('app.expandNode', { title: props.node.title })
            }
            onClick={(event) => {
              event.stopPropagation();
              toggleExpand();
            }}
            onMouseDown={(event) => event.preventDefault()}
          >
            <span class={styles.nodeIcon} aria-hidden="true">
              {props.node.icon ? (
                <IconDisplay icon={props.node.icon} class={styles.sidebarIcon} />
              ) : props.node.has_table && !props.node.has_page ? (
                <TableChartIcon />
              ) : (
                <DescriptionIcon />
              )}
            </span>
            <span
              class={styles.expandIcon}
              classList={{
                [`${styles.expanded}`]: isExpanded(),
                [`${styles.loading}`]: isLoadingChildren(),
              }}
              aria-hidden="true"
            >
              {isLoadingChildren() ? '○' : <ChevronRightIcon />}
            </span>
          </span>
        </Show>
        <button
          ref={(el) => (treeItemRef = el)}
          type="button"
          class={styles.pageButton}
          role="treeitem"
          aria-label={props.node.title}
          data-node-id={props.node.id}
          aria-current={props.selectedId === props.node.id ? 'page' : undefined}
          aria-expanded={mightHaveChildren() ? isExpanded() : undefined}
          aria-owns={isExpanded() && children().length > 0 ? `tree-group-${props.node.id}` : undefined}
          aria-level={props.depth + 1}
          aria-selected={props.selectedId === props.node.id}
          aria-keyshortcuts="Control+V Shift+F10"
          tabIndex={props.focusedNodeId === props.node.id || (!props.focusedNodeId && props.isFirstTreeItem) ? 0 : -1}
          onClick={(event) => {
            event.stopPropagation();
            props.onSelect(props.node);
          }}
          onFocus={() => props.onFocusNode(props.node.id)}
          onKeyDown={handleTreeKeyDown}
        >
          <span class={styles.pageTitleText}>{props.node.title}</span>
        </button>
        <span
          class={styles.hoverDeleteButton}
          data-testid="delete-node-button"
          onClick={(e) => {
            e.stopPropagation();
            requestDelete();
          }}
          title={t('common.delete') || 'Delete'}
          aria-hidden="true"
          onMouseDown={(event) => event.preventDefault()}
        >
          <DeleteIcon />
        </span>
      </div>

      <Show when={showContextMenu()}>
        <ContextMenu
          position={contextMenuPos()}
          actions={contextActions()}
          onAction={(actionId) => {
            switch (actionId) {
              case 'create-subpage':
                props.onCreateChildPage(props.node.id);
                break;
              case 'create-subtable':
                props.onCreateChildTable(props.node.id);
                break;
              case 'history':
                props.onShowHistory?.(props.node.id);
                break;
              case 'move-node':
                props.onStartMove(props.node.id);
                break;
              case 'move-root':
                void props.onMoveNode?.(props.node.id, '0');
                break;
              case 'move-here':
                if (props.movingNodeId) {
                  void props.onMoveNode?.(props.movingNodeId, props.node.id);
                }
                break;
              case 'delete-node':
                requestDelete();
                break;
            }
          }}
          onClose={() => setShowContextMenu(false)}
          trigger={() => treeItemRef}
        />
      </Show>

      <Show when={isExpanded() && children().length > 0}>
        <ul id={`tree-group-${props.node.id}`} class={styles.childList} role="group">
          <For each={children()}>
            {(child) => (
              <SidebarNode
                node={child}
                selectedId={props.selectedId}
                ancestorIds={props.ancestorIds}
                focusedNodeId={props.focusedNodeId}
                movingNodeId={props.movingNodeId}
                onSelect={props.onSelect}
                onFocusNode={props.onFocusNode}
                onStartMove={props.onStartMove}
                onCreateChildPage={props.onCreateChildPage}
                onCreateChildTable={props.onCreateChildTable}
                onFetchChildren={props.onFetchChildren}
                onDeleteNode={props.onDeleteNode}
                onShowHistory={props.onShowHistory}
                onMoveNode={props.onMoveNode}
                depth={props.depth + 1}
                isFirstTreeItem={false}
              />
            )}
          </For>
        </ul>
      </Show>
    </li>
  );
}
