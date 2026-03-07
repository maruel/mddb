// Positioned filter panel for per-column filtering, rendered via Portal.

import { createEffect, createSignal, For, Show, onCleanup, untrack } from 'solid-js';
import { Portal } from 'solid-js/web';
import {
  type Filter,
  type Property,
  type FilterOp,
  FilterOpEquals,
  FilterOpNotEquals,
  FilterOpContains,
  FilterOpNotContains,
  FilterOpStartsWith,
  FilterOpEndsWith,
  FilterOpIsEmpty,
  FilterOpIsNotEmpty,
  FilterOpGreaterThan,
  FilterOpLessThan,
  FilterOpGreaterEqual,
  FilterOpLessEqual,
  PropertyTypeNumber,
  PropertyTypeDate,
  PropertyTypeCheckbox,
  PropertyTypeSelect,
  PropertyTypeMultiSelect,
} from '@sdk/types.gen';
import styles from './FilterPanel.module.css';
import { useI18n } from '../../i18n';

interface FilterPanelProps {
  column: Property;
  position: { x: number; y: number };
  currentFilter: Filter | undefined;
  onApply: (filter: Filter) => void;
  onRemove: () => void;
  onClose: () => void;
}

interface OperatorOption {
  value: FilterOp;
  label: string;
}

const NO_VALUE_OPS: FilterOp[] = [FilterOpIsEmpty, FilterOpIsNotEmpty];

export function FilterPanel(props: FilterPanelProps) {
  const { t } = useI18n();
  let panelRef: HTMLDivElement | undefined;
  const [adjustedPos, setAdjustedPos] = createSignal({ x: 0, y: 0 });

  const defaultOperator = (): FilterOp => {
    const type = props.column.type;
    if (type === PropertyTypeCheckbox || type === PropertyTypeNumber || type === PropertyTypeDate)
      return FilterOpEquals;
    return FilterOpContains;
  };

  const [operator, setOperator] = createSignal<FilterOp>(
    untrack(() => (props.currentFilter?.operator as FilterOp | undefined) ?? defaultOperator())
  );
  const [value, setValue] = createSignal<string>(
    untrack(() => (props.currentFilter?.value !== undefined ? String(props.currentFilter.value) : ''))
  );

  const getOperators = (): OperatorOption[] => {
    const type = props.column.type;
    const base: OperatorOption[] = [
      { value: FilterOpEquals, label: t('table.opEquals') },
      { value: FilterOpNotEquals, label: t('table.opNotEquals') },
    ];

    if (type === PropertyTypeNumber || type === PropertyTypeDate) {
      return [
        ...base,
        { value: FilterOpGreaterThan, label: t('table.opGt') },
        { value: FilterOpGreaterEqual, label: t('table.opGte') },
        { value: FilterOpLessThan, label: t('table.opLt') },
        { value: FilterOpLessEqual, label: t('table.opLte') },
        { value: FilterOpIsEmpty, label: t('table.opIsEmpty') },
        { value: FilterOpIsNotEmpty, label: t('table.opIsNotEmpty') },
      ];
    }

    if (type === PropertyTypeCheckbox) {
      return base;
    }

    if (type === PropertyTypeSelect || type === PropertyTypeMultiSelect) {
      return [
        ...base,
        { value: FilterOpContains, label: t('table.opContains') },
        { value: FilterOpIsEmpty, label: t('table.opIsEmpty') },
        { value: FilterOpIsNotEmpty, label: t('table.opIsNotEmpty') },
      ];
    }

    // text, url, email, phone
    return [
      ...base,
      { value: FilterOpContains, label: t('table.opContains') },
      { value: FilterOpNotContains, label: t('table.opNotContains') },
      { value: FilterOpStartsWith, label: t('table.opStartsWith') },
      { value: FilterOpEndsWith, label: t('table.opEndsWith') },
      { value: FilterOpIsEmpty, label: t('table.opIsEmpty') },
      { value: FilterOpIsNotEmpty, label: t('table.opIsNotEmpty') },
    ];
  };

  const needsValue = () => !NO_VALUE_OPS.includes(operator());

  // Viewport boundary detection
  createEffect(() => {
    if (!panelRef) return;
    const rect = panelRef.getBoundingClientRect();
    const padding = 8;
    let x = props.position.x;
    let y = props.position.y;
    if (x + rect.width > window.innerWidth - padding) {
      x = Math.max(padding, window.innerWidth - rect.width - padding);
    }
    if (y + rect.height > window.innerHeight - padding) {
      y = Math.max(padding, window.innerHeight - rect.height - padding);
    }
    setAdjustedPos({ x, y });
  });

  // Click outside to close
  createEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (panelRef && !panelRef.contains(e.target as Node)) {
        props.onClose();
      }
    };
    const id = setTimeout(() => document.addEventListener('click', handleClickOutside), 0);
    onCleanup(() => {
      clearTimeout(id);
      document.removeEventListener('click', handleClickOutside);
    });
  });

  // Escape to close
  createEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        props.onClose();
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    onCleanup(() => document.removeEventListener('keydown', handleKeyDown));
  });

  const handleApply = () => {
    let filterValue: unknown = value();
    if (needsValue()) {
      if (props.column.type === PropertyTypeNumber) {
        filterValue = value() === '' ? '' : Number(value());
      } else if (props.column.type === PropertyTypeCheckbox) {
        filterValue = value() === 'true';
      }
    }
    const filter: Filter = {
      property: props.column.name,
      operator: operator(),
      value: needsValue() ? filterValue : undefined,
    };
    props.onApply(filter);
    props.onClose();
  };

  return (
    <Portal>
      <div
        ref={(el) => (panelRef = el)}
        class={styles.panel}
        style={{ left: `${adjustedPos().x}px`, top: `${adjustedPos().y}px` }}
        data-testid="filter-panel"
      >
        <div class={styles.row}>
          <select
            class={styles.select}
            value={operator()}
            onChange={(e) => setOperator(e.target.value as FilterOp)}
            data-testid="filter-operator"
          >
            <For each={getOperators()}>{(op) => <option value={op.value}>{op.label}</option>}</For>
          </select>
        </div>
        <Show when={needsValue()}>
          <div class={styles.row}>
            <Show
              when={props.column.type === PropertyTypeCheckbox}
              fallback={
                <input
                  class={styles.input}
                  type={
                    props.column.type === PropertyTypeNumber
                      ? 'number'
                      : props.column.type === PropertyTypeDate
                        ? 'date'
                        : 'text'
                  }
                  value={value()}
                  onInput={(e) => setValue(e.currentTarget.value)}
                  placeholder={t('table.filterValue')}
                  data-testid="filter-value"
                  autofocus
                />
              }
            >
              <select
                class={styles.select}
                value={value()}
                onChange={(e) => setValue(e.currentTarget.value)}
                data-testid="filter-value"
              >
                <option value="true">true</option>
                <option value="false">false</option>
              </select>
            </Show>
          </div>
        </Show>
        <div class={styles.actions}>
          <button class={styles.applyBtn} onClick={handleApply} data-testid="filter-apply">
            {t('table.filterApply')}
          </button>
          <Show when={props.currentFilter}>
            <button
              class={styles.removeBtn}
              onClick={() => {
                props.onRemove();
                props.onClose();
              }}
              data-testid="filter-remove"
            >
              {t('table.removeFilter')}
            </button>
          </Show>
        </div>
      </div>
    </Portal>
  );
}
