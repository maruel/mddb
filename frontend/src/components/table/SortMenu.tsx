// Dropdown menu for managing sort rules on a table view.

import { For, Show, onMount, onCleanup } from 'solid-js';
import { useRecords, DEFAULT_VIEW_ID } from '../../contexts';
import { useI18n } from '../../i18n';
import { SortAsc, type Property, type Sort } from '@sdk/types.gen';
import { availableProperties, addSort, removeSort, toggleSortDirection, changeSortProperty } from './sortUtils';
import styles from './SortMenu.module.css';

import ArrowUpwardIcon from '@material-symbols/svg-400/outlined/arrow_upward.svg?solid';
import ArrowDownwardIcon from '@material-symbols/svg-400/outlined/arrow_downward.svg?solid';
import CloseIcon from '@material-symbols/svg-400/outlined/close.svg?solid';
import AddIcon from '@material-symbols/svg-400/outlined/add.svg?solid';

interface SortMenuProps {
  properties: Property[];
  onClose: () => void;
}

export default function SortMenu(props: SortMenuProps) {
  const { t } = useI18n();
  const { activeSorts, setSorts, updateView, activeViewId } = useRecords();

  // Document-level Escape listener (works regardless of focus location)
  onMount(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') props.onClose();
    };
    document.addEventListener('keydown', handleKeyDown);
    onCleanup(() => document.removeEventListener('keydown', handleKeyDown));
  });

  function applyAndPersist(newSorts: Sort[]) {
    setSorts(newSorts);
    const viewId = activeViewId();
    if (viewId && viewId !== DEFAULT_VIEW_ID) {
      updateView(viewId, { sorts: newSorts });
    }
  }

  function handlePropertyChange(index: number, propertyName: string) {
    const newSorts = changeSortProperty(activeSorts(), index, propertyName);
    if (newSorts) applyAndPersist(newSorts);
  }

  function handleDirectionToggle(index: number) {
    const newSorts = toggleSortDirection(activeSorts(), index);
    if (newSorts) applyAndPersist(newSorts);
  }

  function handleRemove(index: number) {
    applyAndPersist(removeSort(activeSorts(), index));
  }

  function handleAdd() {
    const newSorts = addSort(activeSorts(), props.properties);
    if (newSorts) applyAndPersist(newSorts);
  }

  return (
    <div class={styles.menu} data-testid="sort-menu">
      <Show when={activeSorts().length === 0}>
        <div class={styles.empty}>{t('table.noSorts')}</div>
      </Show>

      <For each={activeSorts()}>
        {(sort, index) => (
          <div class={styles.sortRow} data-testid="sort-row">
            <select
              data-testid="sort-property-select"
              value={sort.property}
              onChange={(e) => handlePropertyChange(index(), e.target.value)}
            >
              <For each={availableProperties(props.properties, activeSorts(), index())}>
                {(prop) => (
                  <option value={prop.name} selected={prop.name === sort.property}>
                    {prop.name}
                  </option>
                )}
              </For>
            </select>

            <button
              class={styles.dirButton}
              data-testid="sort-direction-toggle"
              onClick={() => handleDirectionToggle(index())}
              title={sort.direction === SortAsc ? t('table.ascending') : t('table.descending')}
            >
              <Show when={sort.direction === SortAsc} fallback={<ArrowDownwardIcon />}>
                <ArrowUpwardIcon />
              </Show>
            </button>

            <button
              class={styles.removeButton}
              data-testid="sort-remove"
              onClick={() => handleRemove(index())}
              title={t('common.remove')}
            >
              <CloseIcon />
            </button>
          </div>
        )}
      </For>

      <Show when={availableProperties(props.properties, activeSorts(), -1).length > 0}>
        <button class={styles.addButton} data-testid="add-sort-button" onClick={handleAdd}>
          <AddIcon />
          {t('table.addSort')}
        </button>
      </Show>
    </div>
  );
}
