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
  onCreatePage: () => void;
  onCreateTable: () => void;
  onCreateChildPage: (parentId: string) => void;
  onCreateChildTable: (parentId: string) => void;
  onSelectNode: (node: NodeResponse) => void;
  onCloseMobileSidebar: () => void;
  onFetchChildren?: (nodeId: string) => Promise<NodeResponse[]>;
  onDeleteNode?: (nodeId: string) => void;
  onShowHistory?: (nodeId: string) => void;
}

export default function Sidebar(props: SidebarProps) {
  const { t } = useI18n();

  const navigateTo = (e: MouseEvent, path: string) => {
    e.preventDefault();
    window.history.pushState(null, '', path);
    window.dispatchEvent(new PopStateEvent('popstate'));
    props.onCloseMobileSidebar();
  };

  return (
    <aside class={`${styles.sidebar} ${props.isOpen ? styles.mobileOpen : ''}`}>
      <div class={styles.sidebarHeader}>
        <h2>{t('app.workspace')}</h2>
        <div class={styles.sidebarActions}>
          <button onClick={props.onCreatePage} title={t('app.newPage') || 'New Page'}>
            +P
          </button>
          <button onClick={props.onCreateTable} title={t('app.newTable') || 'New Table'}>
            +D
          </button>
        </div>
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

      <div class={styles.sidebarFooter}>
        <a href="/privacy" onClick={(e) => navigateTo(e, '/privacy')}>
          {t('app.privacyPolicy')}
        </a>
        <span class={styles.footerSeparator}>|</span>
        <a href="/terms" onClick={(e) => navigateTo(e, '/terms')}>
          {t('app.terms')}
        </a>
      </div>
    </aside>
  );
}
