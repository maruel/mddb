// Horizontal tabs for switching between saved table views.

import { createSignal, For, Show } from 'solid-js';
import { useRecords } from '../../contexts';
import { useClickOutside } from '../../composables/useClickOutside';
import { useI18n } from '../../i18n';
import type { View, ViewType } from '@sdk/types.gen';
import { ContextMenu, type ContextMenuAction } from '../shared';
import styles from './ViewTabs.module.css';

import TableRowsIcon from '@material-symbols/svg-400/outlined/table_rows.svg?solid';
import GridGoldenratioIcon from '@material-symbols/svg-400/outlined/grid_goldenratio.svg?solid';
import GridViewIcon from '@material-symbols/svg-400/outlined/grid_view.svg?solid';
import ViewStreamIcon from '@material-symbols/svg-400/outlined/view_stream.svg?solid';
import CalendarMonthIcon from '@material-symbols/svg-400/outlined/calendar_month.svg?solid';
import AddIcon from '@material-symbols/svg-400/outlined/add.svg?solid';
import DeleteIcon from '@material-symbols/svg-400/outlined/delete.svg?solid';

const VIEW_ICONS: Record<ViewType, SolidSVG> = {
  table: TableRowsIcon,
  board: GridGoldenratioIcon,
  gallery: GridViewIcon,
  list: ViewStreamIcon,
  calendar: CalendarMonthIcon,
};

export default function ViewTabs() {
  const { t } = useI18n();
  const { views, activeViewId, setActiveViewId, createView, updateView, deleteView } = useRecords();

  const [showNewViewMenu, setShowNewViewMenu] = createSignal(false);

  // Context menu for right-clicking a tab
  const [tabMenu, setTabMenu] = createSignal<{ view: View; x: number; y: number } | null>(null);

  // Inline rename state
  const [renamingViewId, setRenamingViewId] = createSignal<string | null>(null);
  const [renameValue, setRenameValue] = createSignal('');

  let addWrapperRef: HTMLDivElement | undefined;
  useClickOutside(
    () => addWrapperRef,
    () => setShowNewViewMenu(false)
  );

  const handleTabClick = (viewId: string) => {
    setActiveViewId(viewId);
  };

  const handleNewView = (type: ViewType) => {
    const name = t('table.newView') || 'New View';
    createView(name, type);
    setShowNewViewMenu(false);
  };

  const handleTabContextMenu = (e: MouseEvent, view: View) => {
    e.preventDefault();
    setTabMenu({ view, x: e.clientX, y: e.clientY });
  };

  const getTabActions = (view: View): ContextMenuAction[] => {
    const actions: ContextMenuAction[] = [
      { id: 'rename', label: t('table.renameView') || 'Rename view' },
      { id: 'duplicate', label: t('table.duplicateView') || 'Duplicate view' },
    ];
    if (!view.default) {
      actions.push({
        id: 'delete',
        label: t('common.delete') || 'Delete',
        danger: true,
        separator: true,
      });
    }
    return actions;
  };

  const handleTabAction = (actionId: string) => {
    const state = tabMenu();
    setTabMenu(null);
    if (!state) return;

    switch (actionId) {
      case 'rename':
        setRenameValue(state.view.name);
        setRenamingViewId(state.view.id);
        break;
      case 'duplicate':
        createView(state.view.name + ' (copy)', state.view.type);
        break;
      case 'delete':
        deleteView(state.view.id);
        break;
    }
  };

  const commitRename = () => {
    const viewId = renamingViewId();
    if (!viewId) return;
    const newName = renameValue().trim();
    if (newName) {
      updateView(viewId, { name: newName });
    }
    setRenamingViewId(null);
  };

  return (
    <div class={styles.container}>
      <div class={styles.tabs}>
        <For each={views()}>
          {(view) => {
            const Icon = VIEW_ICONS[view.type] || TableRowsIcon;
            return (
              <div class={styles.tabWrapper}>
                <button
                  class={styles.tab}
                  classList={{ [`${styles.active}`]: view.id === activeViewId() }}
                  onClick={() => handleTabClick(view.id)}
                  onContextMenu={(e) => handleTabContextMenu(e, view)}
                  title={view.name}
                >
                  <span class={styles.icon}>
                    <Icon />
                  </span>
                  <Show when={renamingViewId() === view.id} fallback={<span class={styles.name}>{view.name}</span>}>
                    <input
                      class={styles.renameInput}
                      value={renameValue()}
                      onInput={(e) => setRenameValue(e.target.value)}
                      onClick={(e) => e.stopPropagation()}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') commitRename();
                        if (e.key === 'Escape') setRenamingViewId(null);
                        e.stopPropagation();
                      }}
                      onBlur={commitRename}
                      ref={(el) => setTimeout(() => el?.select(), 0)}
                    />
                  </Show>
                </button>
                <Show when={!view.default}>
                  <button
                    class={styles.deleteBtn}
                    onClick={() => deleteView(view.id)}
                    title={t('common.delete') || 'Delete'}
                    tabIndex={-1}
                  >
                    <DeleteIcon />
                  </button>
                </Show>
              </div>
            );
          }}
        </For>
      </div>

      {/* addWrapper is a sibling of .tabs, not nested inside it, so the dropdown
          is not clipped by .tabs's overflow-x:auto */}
      <div class={styles.addWrapper} ref={(el) => (addWrapperRef = el)}>
        <button
          class={styles.addButton}
          onClick={() => setShowNewViewMenu(!showNewViewMenu())}
          title={t('table.newView') || 'New View'}
          data-testid="add-view-button"
        >
          <AddIcon />
          <span>{t('table.newView') || 'New View'}</span>
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

      {/* Tab right-click context menu */}
      <Show when={tabMenu()}>
        {(state) => (
          <ContextMenu
            position={{ x: state().x, y: state().y }}
            actions={getTabActions(state().view)}
            onAction={handleTabAction}
            onClose={() => setTabMenu(null)}
          />
        )}
      </Show>
    </div>
  );
}
