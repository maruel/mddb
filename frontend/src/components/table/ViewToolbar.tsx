// Toolbar with Sort and Filter buttons for table views.

import { createSignal, Show } from 'solid-js';
import { useRecords } from '../../contexts';
import { useClickOutside } from '../../composables/useClickOutside';
import { useI18n } from '../../i18n';
import SortMenu from './SortMenu';
import type { Property } from '@sdk/types.gen';
import styles from './ViewToolbar.module.css';

import SwapVertIcon from '@material-symbols/svg-400/outlined/swap_vert.svg?solid';
import FilterListIcon from '@material-symbols/svg-400/outlined/filter_list.svg?solid';

interface ViewToolbarProps {
  properties: Property[];
}

export default function ViewToolbar(props: ViewToolbarProps) {
  const { t } = useI18n();
  const { activeSorts } = useRecords();

  const [showSortMenu, setShowSortMenu] = createSignal(false);

  let sortWrapperRef: HTMLDivElement | undefined;
  useClickOutside(
    () => sortWrapperRef,
    () => setShowSortMenu(false)
  );

  return (
    <div class={styles.toolbar}>
      <div class={styles.sortWrapper} ref={(el) => (sortWrapperRef = el)}>
        <button
          class={styles.toolbarButton}
          classList={{ [`${styles.active}`]: activeSorts().length > 0 }}
          onClick={() => setShowSortMenu(!showSortMenu())}
          data-testid="sort-button"
        >
          <SwapVertIcon />
          {t('table.sort')}
          <Show when={activeSorts().length > 0}>
            <span class={styles.badge}>{activeSorts().length}</span>
          </Show>
        </button>

        <Show when={showSortMenu()}>
          <SortMenu properties={props.properties} onClose={() => setShowSortMenu(false)} />
        </Show>
      </div>

      <button class={styles.toolbarButton} disabled data-testid="filter-button">
        <FilterListIcon />
        {t('table.filter')}
      </button>
    </div>
  );
}
