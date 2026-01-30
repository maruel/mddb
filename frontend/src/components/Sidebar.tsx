// Sidebar navigation component containing the workspace tree.

import { For, Show } from 'solid-js';
import SidebarNode from './SidebarNode';
import { useI18n } from '../i18n';
import type { NodeResponse } from '@sdk/types.gen';
import styles from './Sidebar.module.css';

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
}

export default function Sidebar(props: SidebarProps) {
  const { t } = useI18n();

  return (
    <aside class={`${styles.sidebar} ${props.isOpen ? styles.open : ''}`}>
      <div class={styles.sidebarHeader}>
        <h2>{t('app.workspace')}</h2>
        <button
          class={styles.collapseButton}
          onClick={() => props.onCloseMobileSidebar()}
          title={t('app.collapseSidebar') || 'Collapse sidebar'}
        >
          «
        </button>
      </div>
      <div class={styles.sidebarActions}>
        <button onClick={() => props.onCreatePage()} title={t('app.newPage') || 'New Page'} class={styles.actionButton}>
          + {t('app.page')}
        </button>
        <button
          onClick={() => props.onCreateTable()}
          title={t('app.newTable') || 'New Table'}
          class={styles.actionButton}
        >
          + {t('app.table')}
        </button>
      </div>
      <Show when={props.loading && props.nodes.length === 0}>
        <p class={styles.loading}>{t('common.loading')}</p>
      </Show>

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
    </aside>
  );
}
