import { createSignal, For, Show } from 'solid-js';
import type { NodeResponse } from '../types.gen';
import styles from '../App.module.css';

interface SidebarNodeResponseProps {
  node: NodeResponse;
  selectedId: string | null;
  onSelect: (node: NodeResponse) => void;
  depth: number;
}

export default function SidebarNodeResponse(props: SidebarNodeResponseProps) {
  const [isExpanded, setIsExpanded] = createSignal(true);

  const toggleExpand = (e: MouseEvent) => {
    e.stopPropagation();
    setIsExpanded(!isExpanded());
  };

  return (
    <li class={styles.sidebarNodeResponseWrapper}>
      <div
        class={styles.pageItem}
        classList={{ [`${styles.active}`]: props.selectedId === props.node.id }}
        style={{ 'padding-left': `${props.depth * 12 + 8}px` }}
        onClick={() => props.onSelect(props.node)}
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

      <Show when={isExpanded() && props.node.children?.length}>
        <ul class={styles.childList}>
          <For each={props.node.children?.filter((c): c is NodeResponse => !!c)}>
            {(child) => (
              <SidebarNodeResponse
                node={child}
                selectedId={props.selectedId}
                onSelect={props.onSelect}
                depth={props.depth + 1}
              />
            )}
          </For>
        </ul>
      </Show>
    </li>
  );
}
