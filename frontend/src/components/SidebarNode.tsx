// Recursive component for rendering navigation tree nodes in the sidebar.

import { createSignal, For, Show, onMount } from 'solid-js';
import { useI18n } from '../i18n';
import type { NodeResponse } from '../types.gen';
import styles from './SidebarNode.module.css';

interface SidebarNodeResponseProps {
  node: NodeResponse;
  selectedId: string | null;
  onSelect: (node: NodeResponse) => void;
  onCreateChildPage: (parentId: string) => void;
  onCreateChildTable: (parentId: string) => void;
  depth: number;
}

export default function SidebarNodeResponse(props: SidebarNodeResponseProps) {
  const { t } = useI18n();
  const [isExpanded, setIsExpanded] = createSignal(true);
  const [showContextMenu, setShowContextMenu] = createSignal(false);
  const [contextMenuPos, setContextMenuPos] = createSignal({ x: 0, y: 0 });

  onMount(() => {
    const handleClickAway = () => {
      setShowContextMenu(false);
    };
    document.addEventListener('click', handleClickAway);
    return () => document.removeEventListener('click', handleClickAway);
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
    <li class={styles.sidebarNodeWrapper}>
      <div
        class={styles.pageItem}
        classList={{ [`${styles.active}`]: props.selectedId === props.node.id }}
        style={{ 'padding-left': `${props.depth * 12 + 8}px` }}
        onClick={() => props.onSelect(props.node)}
        onContextMenu={handleContextMenu}
      >
        <span
          class={styles.expandIcon}
          classList={{
            [`${styles.expanded}`]: isExpanded(),
            [`${styles.hidden}`]: !props.node.children?.length,
          }}
          onClick={toggleExpand}
        >
          ▶
        </span>
        <span class={styles.nodeIcon}>{props.node.type === 'table' ? '📊' : '📄'}</span>
        <span class={styles.pageTitleText}>{props.node.title}</span>
      </div>

      <Show when={showContextMenu()}>
        <div
          class={styles.contextMenu}
          style={{ left: `${contextMenuPos().x}px`, top: `${contextMenuPos().y}px` }}
          onClick={(e) => e.stopPropagation()}
        >
          <button
            class={styles.contextMenuItem}
            onClick={() => {
              props.onCreateChildPage(props.node.id);
              setShowContextMenu(false);
            }}
          >
            📄 {t('app.createSubPage')}
          </button>
          <button
            class={styles.contextMenuItem}
            onClick={() => {
              props.onCreateChildTable(props.node.id);
              setShowContextMenu(false);
            }}
          >
            📊 {t('app.createSubTable')}
          </button>
        </div>
      </Show>

      <Show when={isExpanded() && props.node.children?.length}>
        <ul class={styles.childList}>
          <For each={props.node.children?.filter((c): c is NodeResponse => !!c)}>
            {(child) => (
              <SidebarNodeResponse
                node={child}
                selectedId={props.selectedId}
                onSelect={props.onSelect}
                onCreateChildPage={props.onCreateChildPage}
                onCreateChildTable={props.onCreateChildTable}
                depth={props.depth + 1}
              />
            )}
          </For>
        </ul>
      </Show>
    </li>
  );
}
