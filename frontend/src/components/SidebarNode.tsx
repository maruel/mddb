// Recursive component for rendering navigation tree nodes in the sidebar.
// Supports lazy loading of children with pre-fetching one level ahead.

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
  onFetchChildren?: (nodeId: string) => Promise<NodeResponse[]>;
  depth: number;
  // Children loaded from parent's pre-fetch
  prefetchedChildren?: Map<string, NodeResponse[]>;
}

export default function SidebarNodeResponse(props: SidebarNodeResponseProps) {
  const { t } = useI18n();
  // Start collapsed by default, except root level (depth 0)
  const [isExpanded, setIsExpanded] = createSignal(props.depth === 0);
  const [showContextMenu, setShowContextMenu] = createSignal(false);
  const [contextMenuPos, setContextMenuPos] = createSignal({ x: 0, y: 0 });
  const [loadedChildren, setLoadedChildren] = createSignal<NodeResponse[] | null>(null);
  const [isLoadingChildren, setIsLoadingChildren] = createSignal(false);
  const [prefetchCache] = createSignal(new Map<string, NodeResponse[]>());

  // Use prefetched or loaded children
  const children = () => {
    // Check prefetched cache from parent first
    const prefetched = props.prefetchedChildren?.get(props.node.id);
    if (prefetched) return prefetched;

    // Use locally loaded children if available
    const loaded = loadedChildren();
    if (loaded !== null) return loaded;

    return [];
  };

  // Check if this node might have children
  const mightHaveChildren = () => {
    // If we've loaded children, check if there are any
    const loaded = loadedChildren();
    if (loaded !== null) return loaded.length > 0;

    // Check prefetched cache
    const prefetched = props.prefetchedChildren?.get(props.node.id);
    if (prefetched) return prefetched.length > 0;

    // Backend sets has_children to indicate node has children not yet loaded
    return props.node.has_children === true;
  };

  // Fetch children for this node
  const fetchChildren = async () => {
    if (!props.onFetchChildren || loadedChildren() !== null || isLoadingChildren()) return;

    setIsLoadingChildren(true);
    try {
      const fetchedChildren = await props.onFetchChildren(props.node.id);
      setLoadedChildren(fetchedChildren);

      // Pre-fetch grandchildren (one level ahead)
      prefetchGrandchildren(fetchedChildren);
    } catch (err) {
      console.error('Failed to load children:', err);
      setLoadedChildren([]); // Mark as loaded (empty) to prevent retries
    } finally {
      setIsLoadingChildren(false);
    }
  };

  onMount(() => {
    const handleClickAway = () => {
      setShowContextMenu(false);
    };
    document.addEventListener('click', handleClickAway);

    // When node becomes visible, ensure children are loaded and grandchildren pre-fetched
    if (mightHaveChildren() && props.onFetchChildren) {
      // Check if children are already prefetched by parent
      const prefetched = props.prefetchedChildren?.get(props.node.id);
      if (prefetched) {
        // Use prefetched data and pre-fetch grandchildren
        setLoadedChildren(prefetched);
        prefetchGrandchildren(prefetched);
      } else if (loadedChildren() === null) {
        // Not prefetched, fetch now
        fetchChildren();
      }
    }

    return () => document.removeEventListener('click', handleClickAway);
  });

  // Pre-fetch grandchildren when expanding
  const prefetchGrandchildren = (childNodes: NodeResponse[]) => {
    if (!props.onFetchChildren) return;

    // Fetch children for each child node in parallel (non-blocking)
    for (const child of childNodes) {
      if (!prefetchCache().has(child.id) && child.has_children === true) {
        props
          .onFetchChildren(child.id)
          .then((grandchildren) => {
            prefetchCache().set(child.id, grandchildren);
          })
          .catch(() => {
            // Ignore pre-fetch errors
          });
      }
    }
  };

  const toggleExpand = async (e: MouseEvent) => {
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
          class={styles.expandIcon}
          classList={{
            [`${styles.expanded}`]: isExpanded(),
            [`${styles.hidden}`]: !mightHaveChildren(),
            [`${styles.loading}`]: isLoadingChildren(),
          }}
          onClick={toggleExpand}
        >
          {isLoadingChildren() ? '○' : '▶'}
        </span>
        <span class={styles.nodeIcon}>{props.node.has_table && !props.node.has_page ? '📊' : '📄'}</span>
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

      <Show when={isExpanded() && children().length > 0}>
        <ul class={styles.childList}>
          <For each={children()}>
            {(child) => (
              <SidebarNodeResponse
                node={child}
                selectedId={props.selectedId}
                onSelect={props.onSelect}
                onCreateChildPage={props.onCreateChildPage}
                onCreateChildTable={props.onCreateChildTable}
                onFetchChildren={props.onFetchChildren}
                depth={props.depth + 1}
                prefetchedChildren={prefetchCache()}
              />
            )}
          </For>
        </ul>
      </Show>
    </li>
  );
}
