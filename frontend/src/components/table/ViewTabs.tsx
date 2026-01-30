// Horizontal tabs for switching between saved table views.

import { createSignal, For, Show } from 'solid-js';
import { useRecords } from '../../contexts';
import { useI18n } from '../../i18n';
import type { View, ViewType } from '@sdk/types.gen';
import styles from './ViewTabs.module.css';

// Icons for each view type
const VIEW_ICONS: Record<ViewType, string> = {
  table: '☰',
  board: '☷',
  gallery: '▦',
  list: '☰',
  calendar: '📅',
};

export default function ViewTabs() {
  const { t } = useI18n();
  const { views, activeViewId, setActiveViewId, createView, deleteView } = useRecords();

  const [showNewViewMenu, setShowNewViewMenu] = createSignal(false);
  const [contextMenuView, setContextMenuView] = createSignal<View | null>(null);
  const [contextMenuPos, setContextMenuPos] = createSignal<{ x: number; y: number } | null>(null);

  const handleTabClick = (viewId: string) => {
    setActiveViewId(viewId);
  };

  const handleNewView = (type: ViewType) => {
    const name = t('table.newView') || 'New View';
    createView(name, type);
    setShowNewViewMenu(false);
  };

  const handleContextMenu = (e: MouseEvent, view: View) => {
    e.preventDefault();
    setContextMenuView(view);
    setContextMenuPos({ x: e.clientX, y: e.clientY });
  };

  const handleDeleteView = () => {
    const view = contextMenuView();
    if (view) {
      deleteView(view.id);
    }
    setContextMenuView(null);
    setContextMenuPos(null);
  };

  const closeContextMenu = () => {
    setContextMenuView(null);
    setContextMenuPos(null);
  };

  // Close menus when clicking outside
  const handleClickOutside = () => {
    setShowNewViewMenu(false);
    closeContextMenu();
  };

  return (
    <div class={styles.container}>
      <div class={styles.tabs}>
        <For each={views()}>
          {(view) => (
            <button
              class={styles.tab}
              classList={{ [`${styles.active}`]: view.id === activeViewId() }}
              onClick={() => handleTabClick(view.id)}
              onContextMenu={(e) => handleContextMenu(e, view)}
              title={view.name}
            >
              <span class={styles.icon}>{VIEW_ICONS[view.type] || '☰'}</span>
              <span class={styles.name}>{view.name}</span>
              <Show when={view.default}>
                <span class={styles.defaultBadge}>{t('table.defaultView') || 'Default'}</span>
              </Show>
            </button>
          )}
        </For>

        <div class={styles.addWrapper}>
          <button
            class={styles.addButton}
            onClick={() => setShowNewViewMenu(!showNewViewMenu())}
            title={t('table.newView') || 'New View'}
          >
            +
          </button>

          <Show when={showNewViewMenu()}>
            <div class={styles.dropdown}>
              <button onClick={() => handleNewView('table')}>
                <span class={styles.icon}>{VIEW_ICONS.table}</span>
                {t('table.table')}
              </button>
              <button onClick={() => handleNewView('list')}>
                <span class={styles.icon}>{VIEW_ICONS.list}</span>
                {t('table.list')}
              </button>
              <button onClick={() => handleNewView('gallery')}>
                <span class={styles.icon}>{VIEW_ICONS.gallery}</span>
                {t('table.gallery')}
              </button>
              <button onClick={() => handleNewView('board')}>
                <span class={styles.icon}>{VIEW_ICONS.board}</span>
                {t('table.board')}
              </button>
            </div>
          </Show>
        </div>
      </div>

      {/* Context menu for view actions */}
      <Show when={contextMenuPos()}>
        {(pos) => (
          <Show when={contextMenuView()}>
            <div class={styles.overlay} onClick={handleClickOutside} />
            <div class={styles.contextMenu} style={{ left: `${pos().x}px`, top: `${pos().y}px` }}>
              <button class={styles.deleteAction} onClick={handleDeleteView}>
                {t('common.delete')}
              </button>
            </div>
          </Show>
        )}
      </Show>

      {/* Overlay for dropdown */}
      <Show when={showNewViewMenu()}>
        <div class={styles.overlay} onClick={handleClickOutside} />
      </Show>
    </div>
  );
}
