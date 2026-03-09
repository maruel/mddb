// Dropdown UI for adding a new column to a table.

import { createSignal, Show, For } from 'solid-js';
import type { Property, PropertyType, SelectOption } from '@sdk/types.gen';
import { useI18n } from '../../i18n';
import { OPTION_COLORS } from './SelectOptionsEditor';
import styles from './AddColumnDropdown.module.css';

// Column types available for adding
const COLUMN_TYPES: { type: PropertyType; labelKey: string }[] = [
  { type: 'text', labelKey: 'table.typeText' },
  { type: 'number', labelKey: 'table.typeNumber' },
  { type: 'checkbox', labelKey: 'table.typeCheckbox' },
  { type: 'date', labelKey: 'table.typeDate' },
  { type: 'select', labelKey: 'table.typeSelect' },
  { type: 'multi_select', labelKey: 'table.typeMultiSelect' },
  { type: 'user', labelKey: 'table.typeUser' },
  { type: 'url', labelKey: 'table.typeUrl' },
  { type: 'email', labelKey: 'table.typeEmail' },
];

function genOptionId(existing: SelectOption[]): string {
  const ids = new Set(existing.map((o) => o.id));
  let id: string;
  do {
    id = crypto.randomUUID().slice(0, 8);
  } while (ids.has(id));
  return id;
}

export interface AddColumnDropdownProps {
  onAddColumn: (column: Property) => void;
}

/**
 * Dropdown component for adding a new column to a table.
 * For select/multi_select columns, shows an inline option list before confirming.
 */
export function AddColumnDropdown(props: AddColumnDropdownProps) {
  const { t } = useI18n();
  const [showDropdown, setShowDropdown] = createSignal(false);
  const [newColumnName, setNewColumnName] = createSignal('');
  const [newColumnType, setNewColumnType] = createSignal<PropertyType>('text');
  const [inlineOptions, setInlineOptions] = createSignal<SelectOption[]>([]);
  const [openSwatchFor, setOpenSwatchFor] = createSignal<string | null>(null);

  const isSelectType = () => newColumnType() === 'select' || newColumnType() === 'multi_select';

  const handleTypeChange = (type: PropertyType) => {
    setNewColumnType(type);
    setInlineOptions([]);
    setOpenSwatchFor(null);
  };

  const handleAddOption = () => {
    const newOpt: SelectOption = { id: genOptionId(inlineOptions()), name: '' };
    setInlineOptions([...inlineOptions(), newOpt]);
  };

  const handleOptionRename = (id: string, name: string) => {
    setInlineOptions(inlineOptions().map((o) => (o.id === id ? { ...o, name } : o)));
  };

  const handleOptionRecolor = (id: string, color: string) => {
    const c = color === '#ffffff' ? undefined : color;
    setInlineOptions(inlineOptions().map((o) => (o.id === id ? { ...o, color: c } : o)));
    setOpenSwatchFor(null);
  };

  const handleDeleteOption = (id: string) => {
    setInlineOptions(inlineOptions().filter((o) => o.id !== id));
  };

  const handleAddColumn = () => {
    const name = newColumnName().trim();
    if (!name) return;

    const newColumn: Property = {
      name,
      type: newColumnType(),
      required: false,
      options: isSelectType() ? inlineOptions().filter((o) => o.name.trim()) : undefined,
    };

    props.onAddColumn(newColumn);
    setNewColumnName('');
    setNewColumnType('text');
    setInlineOptions([]);
    setOpenSwatchFor(null);
    setShowDropdown(false);
  };

  const handleCancel = () => {
    setNewColumnName('');
    setNewColumnType('text');
    setInlineOptions([]);
    setOpenSwatchFor(null);
    setShowDropdown(false);
  };

  return (
    <div class={styles.addColumnCell}>
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
              onChange={(e) => handleTypeChange(e.target.value as PropertyType)}
              class={styles.columnTypeSelect}
            >
              <For each={COLUMN_TYPES}>{(ct) => <option value={ct.type}>{t(ct.labelKey) || ct.type}</option>}</For>
            </select>

            <Show when={isSelectType()}>
              <div class={styles.inlineOptions}>
                <For each={inlineOptions()}>
                  {(opt) => {
                    const isSwatchOpen = () => openSwatchFor() === opt.id;
                    return (
                      <div class={styles.inlineOptionRow}>
                        <div class={styles.inlineSwatchWrapper}>
                          <button
                            class={styles.inlineSwatchBtn}
                            style={opt.color ? { background: opt.color } : {}}
                            onClick={() => setOpenSwatchFor(isSwatchOpen() ? null : opt.id)}
                            aria-label="Change color"
                          />
                          <Show when={isSwatchOpen()}>
                            <div class={styles.inlineSwatchPicker}>
                              <For each={OPTION_COLORS}>
                                {(color) => (
                                  <button
                                    class={styles.inlineSwatchChoice}
                                    style={
                                      color === '#ffffff'
                                        ? {
                                            background: 'var(--c-bg-hover)',
                                            border: '1px solid var(--c-border)',
                                          }
                                        : { background: color }
                                    }
                                    onClick={() => handleOptionRecolor(opt.id, color)}
                                    aria-label={color}
                                  />
                                )}
                              </For>
                            </div>
                          </Show>
                        </div>
                        <input
                          type="text"
                          class={styles.inlineOptionInput}
                          value={opt.name}
                          placeholder={t('table.optionPlaceholder') || 'Option name'}
                          onInput={(e) => handleOptionRename(opt.id, e.currentTarget.value)}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter') e.currentTarget.blur();
                          }}
                        />
                        <button
                          class={styles.inlineOptionDelete}
                          onClick={() => handleDeleteOption(opt.id)}
                          aria-label={t('table.deleteOption') || 'Delete'}
                        >
                          ×
                        </button>
                      </div>
                    );
                  }}
                </For>
                <button class={styles.inlineAddOptionBtn} onClick={handleAddOption}>
                  + {t('table.addOption') || 'Add an option'}
                </button>
              </div>
            </Show>

            <div class={styles.addColumnActions}>
              <button class={styles.addColumnConfirm} onClick={handleAddColumn}>
                {t('common.confirm') || 'Add'}
              </button>
              <button class={styles.addColumnCancel} onClick={handleCancel}>
                {t('common.cancel') || 'Cancel'}
              </button>
            </div>
          </div>
        </Show>
      </div>
    </div>
  );
}
