// Dropdown UI for adding a new column to a table.

import { createSignal, Show, For } from 'solid-js';
import type { Property, PropertyType } from '@sdk/types.gen';
import { useI18n } from '../../i18n';
import styles from './AddColumnDropdown.module.css';

// Column types available for adding
const COLUMN_TYPES: { type: PropertyType; labelKey: string }[] = [
  { type: 'text', labelKey: 'table.typeText' },
  { type: 'number', labelKey: 'table.typeNumber' },
  { type: 'checkbox', labelKey: 'table.typeCheckbox' },
  { type: 'date', labelKey: 'table.typeDate' },
  { type: 'select', labelKey: 'table.typeSelect' },
  { type: 'url', labelKey: 'table.typeUrl' },
  { type: 'email', labelKey: 'table.typeEmail' },
];

export interface AddColumnDropdownProps {
  onAddColumn: (column: Property) => void;
}

/**
 * Dropdown component for adding a new column to a table.
 * Shows a + button that expands into a form for column name and type.
 */
export function AddColumnDropdown(props: AddColumnDropdownProps) {
  const { t } = useI18n();
  const [showDropdown, setShowDropdown] = createSignal(false);
  const [newColumnName, setNewColumnName] = createSignal('');
  const [newColumnType, setNewColumnType] = createSignal<PropertyType>('text');

  const handleAddColumn = () => {
    const name = newColumnName().trim();
    if (!name) return;

    const newColumn: Property = {
      name,
      type: newColumnType(),
      required: false,
    };

    props.onAddColumn(newColumn);
    setNewColumnName('');
    setNewColumnType('text');
    setShowDropdown(false);
  };

  return (
    <th class={styles.addColumnCell}>
      <div class={styles.addColumnWrapper}>
        <button
          class={styles.addColumnBtn}
          onClick={() => setShowDropdown(!showDropdown())}
          title={t('table.addColumn') || 'Add Column'}
        >
          +
        </button>
        <Show when={showDropdown()}>
          <div class={styles.addColumnDropdown}>
            <input
              type="text"
              placeholder={t('table.columnName') || 'Column Name'}
              value={newColumnName()}
              onInput={(e) => setNewColumnName(e.target.value)}
              class={styles.columnNameInput}
              autofocus
            />
            <select
              value={newColumnType()}
              onChange={(e) => setNewColumnType(e.target.value as PropertyType)}
              class={styles.columnTypeSelect}
            >
              <For each={COLUMN_TYPES}>{(ct) => <option value={ct.type}>{t(ct.labelKey) || ct.type}</option>}</For>
            </select>
            <div class={styles.addColumnActions}>
              <button class={styles.addColumnConfirm} onClick={handleAddColumn}>
                {t('common.confirm') || 'Add'}
              </button>
              <button class={styles.addColumnCancel} onClick={() => setShowDropdown(false)}>
                {t('common.cancel') || 'Cancel'}
              </button>
            </div>
          </div>
        </Show>
      </div>
    </th>
  );
}
