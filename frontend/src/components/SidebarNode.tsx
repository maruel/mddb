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

// Module-level drag state (imperative, not reactive)
let dragState: { nodeId: string; element: HTMLElement } | null = null;

interface SidebarNodeProps {
  node: NodeResponse;
  selectedId: string | null;
  ancestorIds: string[];
  onSelect: (node: NodeResponse) => void;
  onCreateChildPage: (parentId: string) => void;
  onCreateChildTable: (parentId: string) => void;
  onFetchChildren?: (nodeId: string) => Promise<void>;
  onDeleteNode?: (nodeId: string) => void;
  onShowHistory?: (nodeId: string) => void;
  onMoveNode?: (nodeId: string, newParentId: string) => void;
  depth: number;
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

  const toggleExpand = (e: MouseEvent) => {
    e.stopPropagation();
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
      props.onMoveNode?.(dragState.nodeId, props.node.id);
    }
  };

  return (
    <li ref={liRef} class={styles.sidebarNodeWrapper} data-testid={`sidebar-node-${props.node.id}`}>
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
        <span
          class={styles.iconSlot}
          classList={{ [`${styles.hasChildren}`]: mightHaveChildren() }}
          onClick={(e) => mightHaveChildren() && toggleExpand(e)}
          data-testid={`expand-icon-${props.node.id}`}
        >
          <span class={styles.nodeIcon}>
            {props.node.has_table && !props.node.has_page ? <TableChartIcon /> : <DescriptionIcon />}
          </span>
          <span
            class={styles.expandIcon}
            classList={{
              [`${styles.expanded}`]: isExpanded(),
              [`${styles.loading}`]: isLoadingChildren(),
            }}
          >
            {isLoadingChildren() ? '○' : <ChevronRightIcon />}
          </span>
        </span>
        <span class={styles.pageTitleText}>{props.node.title}</span>
        <button
          class={styles.hoverDeleteButton}
          data-testid="delete-node-button"
          onClick={(e) => {
            e.stopPropagation();
            props.onDeleteNode?.(props.node.id);
          }}
          title={t('common.delete') || 'Delete'}
        >
          <DeleteIcon />
        </button>
      </div>

      <Show when={showContextMenu()}>
        <ContextMenu
          position={contextMenuPos()}
          actions={
            [
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
            ] as ContextMenuAction[]
          }
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
            }
          }}
          onClose={() => setShowContextMenu(false)}
        />
      </Show>

      <Show when={isExpanded() && children().length > 0}>
        <ul class={styles.childList}>
          <For each={children()}>
            {(child) => (
              <SidebarNode
                node={child}
                selectedId={props.selectedId}
                ancestorIds={props.ancestorIds}
                onSelect={props.onSelect}
                onCreateChildPage={props.onCreateChildPage}
                onCreateChildTable={props.onCreateChildTable}
                onFetchChildren={props.onFetchChildren}
                onDeleteNode={props.onDeleteNode}
                onShowHistory={props.onShowHistory}
                onMoveNode={props.onMoveNode}
                depth={props.depth + 1}
              />
            )}
          </For>
        </ul>
      </Show>
    </li>
  );
}
