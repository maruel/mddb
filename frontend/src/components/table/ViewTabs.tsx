// Horizontal tabs for switching between saved table views.

import { createSignal, For, Show } from 'solid-js';
import { useRecords } from '../../contexts';
import { useI18n } from '../../i18n';
import type { View, ViewType } from '@sdk/types.gen';
import styles from './ViewTabs.module.css';

import TableRowsIcon from '@material-symbols/svg-400/outlined/table_rows.svg?solid';
import GridGoldenratioIcon from '@material-symbols/svg-400/outlined/grid_goldenratio.svg?solid';
import GridViewIcon from '@material-symbols/svg-400/outlined/grid_view.svg?solid';
import ViewStreamIcon from '@material-symbols/svg-400/outlined/view_stream.svg?solid';
import CalendarMonthIcon from '@material-symbols/svg-400/outlined/calendar_month.svg?solid';
import AddIcon from '@material-symbols/svg-400/outlined/add.svg?solid';

// Icons for each view type
const VIEW_ICONS: Record<ViewType, SolidSVG> = {
  table: TableRowsIcon,
  board: GridGoldenratioIcon,
  gallery: GridViewIcon,
  list: ViewStreamIcon,
  calendar: CalendarMonthIcon,
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
          {(view) => {
            const Icon = VIEW_ICONS[view.type] || TableRowsIcon;
            return (
              <button
                class={styles.tab}
                classList={{ [`${styles.active}`]: view.id === activeViewId() }}
                onClick={() => handleTabClick(view.id)}
                onContextMenu={(e) => handleContextMenu(e, view)}
                title={view.name}
              >
                <span class={styles.icon}>
                  <Icon />
                </span>
                <span class={styles.name}>{view.name}</span>
                <Show when={view.default}>
                  <span class={styles.defaultBadge}>{t('table.defaultView') || 'Default'}</span>
                </Show>
              </button>
            );
          }}
        </For>

        <div class={styles.addWrapper}>
          <button
            class={styles.addButton}
            onClick={() => setShowNewViewMenu(!showNewViewMenu())}
            title={t('table.newView') || 'New View'}
            data-testid="add-view-button"
          >
            <AddIcon />
          </button>

          <Show when={showNewViewMenu()}>
            <div class={styles.dropdown} data-testid="view-type-menu">
              <button onClick={() => handleNewView('table')} data-testid="view-type-table">
                <span class={styles.icon}>
                  <TableRowsIcon />
                </span>
                {t('table.table')}
              </button>
              <button onClick={() => handleNewView('list')} data-testid="view-type-list">
                <span class={styles.icon}>
                  <ViewStreamIcon />
                </span>
                {t('table.list')}
              </button>
              <button onClick={() => handleNewView('gallery')} data-testid="view-type-gallery">
                <span class={styles.icon}>
                  <GridViewIcon />
                </span>
                {t('table.gallery')}
              </button>
              <button onClick={() => handleNewView('board')} data-testid="view-type-board">
                <span class={styles.icon}>
                  <GridGoldenratioIcon />
                </span>
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
