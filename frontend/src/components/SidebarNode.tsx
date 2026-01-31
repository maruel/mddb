// Recursive component for rendering navigation tree nodes in the sidebar.

import { createSignal, createEffect, For, Show, on, untrack } from 'solid-js';
import { useI18n } from '../i18n';
import { useClickOutside } from '../composables/useClickOutside';
import type { NodeResponse } from '@sdk/types.gen';
import styles from './SidebarNode.module.css';

import DescriptionIcon from '@material-symbols/svg-400/outlined/description.svg?solid';
import TableChartIcon from '@material-symbols/svg-400/outlined/table_chart.svg?solid';
import ChevronRightIcon from '@material-symbols/svg-400/outlined/chevron_right.svg?solid';
import DeleteIcon from '@material-symbols/svg-400/outlined/delete.svg?solid';
import HistoryIcon from '@material-symbols/svg-400/outlined/history.svg?solid';

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
  let contextMenuRef: HTMLDivElement | undefined;

  useClickOutside(
    () => contextMenuRef,
    () => setShowContextMenu(false)
  );

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
  let hasAutoExpanded = false;
  createEffect(
    on(
      () => props.ancestorIds.length,
      (length, prevLength) => {
        const wasEmpty = prevLength === undefined || prevLength === 0;
        if (wasEmpty && length > 0 && !hasAutoExpanded && props.ancestorIds.includes(props.node.id)) {
          hasAutoExpanded = true;
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

  return (
    <li class={styles.sidebarNodeWrapper} data-testid={`sidebar-node-${props.node.id}`}>
      <div
        class={styles.pageItem}
        classList={{ [`${styles.active}`]: props.selectedId === props.node.id }}
        style={{ 'padding-left': `${props.depth * 12 + 8}px` }}
        onClick={() => props.onSelect(props.node)}
        onContextMenu={handleContextMenu}
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
        <div
          class={styles.contextMenu}
          ref={contextMenuRef}
          style={{ left: `${contextMenuPos().x}px`, top: `${contextMenuPos().y}px` }}
          onClick={(e) => e.stopPropagation()}
        >
          <button
            class={styles.contextMenuItem}
            data-testid="create-subpage-button"
            onClick={() => {
              props.onCreateChildPage(props.node.id);
              setShowContextMenu(false);
            }}
          >
            <DescriptionIcon /> {t('app.createSubPage')}
          </button>
          <button
            class={styles.contextMenuItem}
            data-testid="create-subtable-button"
            onClick={() => {
              props.onCreateChildTable(props.node.id);
              setShowContextMenu(false);
            }}
          >
            <TableChartIcon /> {t('app.createSubTable')}
          </button>
          <div class={styles.contextMenuDivider} />
          <button
            class={styles.contextMenuItem}
            data-testid="show-history-button"
            onClick={() => {
              props.onShowHistory?.(props.node.id);
              setShowContextMenu(false);
            }}
          >
            <HistoryIcon /> {t('editor.history')}
          </button>
        </div>
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
                depth={props.depth + 1}
              />
            )}
          </For>
        </ul>
      </Show>
    </li>
  );
}
