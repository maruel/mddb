// Shared always-editable field input for card-style views (gallery, grid).

import { For, Switch, Match, createSignal } from 'solid-js';
import {
  type DataRecordResponse,
  type Property,
  PropertyTypeCheckbox,
  PropertyTypeSelect,
  PropertyTypeMultiSelect,
  PropertyTypeNumber,
  PropertyTypeDate,
  PropertyTypeURL,
  PropertyTypeEmail,
  PropertyTypePhone,
} from '@sdk/types.gen';
import { updateRecordField, handleEnterBlur, getFieldValue } from './tableUtils';
import { useClickOutside } from '../../composables/useClickOutside';
import styles from './FieldEditor.module.css';

interface FieldEditorProps {
  record: DataRecordResponse;
  column: Property;
  onUpdate?: (id: string, data: Record<string, unknown>) => void;
}

function MultiSelectEditor(props: { column: Property; value: string; onSave: (v: string) => void }) {
  const [open, setOpen] = createSignal(false);
  let wrapperRef: HTMLDivElement | undefined;

  useClickOutside(
    () => wrapperRef,
    () => setOpen(false)
  );

  const selectedIds = () =>
    props.value
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);

  const toggle = (id: string) => {
    const current = selectedIds();
    const next = current.includes(id) ? current.filter((x) => x !== id) : [...current, id];
    props.onSave(next.join(','));
  };

  const unselectedOptions = () => (props.column.options ?? []).filter((o) => !selectedIds().includes(o.id));

  const optionName = (id: string) => props.column.options?.find((o) => o.id === id)?.name ?? id;

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Escape') setOpen(false);
  };

  return (
    <div
      class={styles.multiSelectWrapper}
      ref={(el) => (wrapperRef = el)}
      onKeyDown={handleKeyDown}
      onClick={() => setOpen(true)}
    >
      <div class={styles.chipList}>
        <For each={selectedIds()}>
          {(id) => (
            <span class={styles.chip}>
              {optionName(id)}
              <button
                class={styles.chipRemove}
                onClick={(e) => {
                  e.stopPropagation();
                  toggle(id);
                }}
                type="button"
                aria-label={`Remove ${optionName(id)}`}
              >
                ×
              </button>
            </span>
          )}
        </For>
      </div>
      <Switch>
        <Match when={open() && unselectedOptions().length > 0}>
          <div class={styles.optionsList}>
            <For each={unselectedOptions()}>
              {(opt) => (
                <div
                  class={styles.optionItem}
                  onClick={(e) => {
                    e.stopPropagation();
                    toggle(opt.id);
                  }}
                >
                  {opt.name}
                </div>
              )}
            </For>
          </div>
        </Match>
      </Switch>
    </div>
  );
}

export function FieldEditor(props: FieldEditorProps) {
  const value = () => getFieldValue(props.record, props.column.name);
  const save = (v: string) => updateRecordField(props.record, props.column.name, v, props.onUpdate);

  return (
    <Switch
      fallback={
        <input
          type="text"
          value={value()}
          onBlur={(e) => save(e.currentTarget.value)}
          onKeyDown={handleEnterBlur}
          class={styles.input}
        />
      }
    >
      <Match when={props.column.type === PropertyTypeCheckbox}>
        <input
          type="checkbox"
          checked={value() === 'true'}
          onChange={(e) => save(String(e.currentTarget.checked))}
          class={styles.checkbox}
        />
      </Match>

      <Match when={props.column.type === PropertyTypeSelect}>
        <select value={value()} onChange={(e) => save(e.currentTarget.value)} class={styles.select}>
          <option value="">--</option>
          <For each={props.column.options}>{(opt) => <option value={opt.id}>{opt.name}</option>}</For>
        </select>
      </Match>

      <Match when={props.column.type === PropertyTypeMultiSelect}>
        <MultiSelectEditor column={props.column} value={value()} onSave={save} />
      </Match>

      <Match when={props.column.type === PropertyTypeNumber}>
        <input
          type="number"
          value={value()}
          onBlur={(e) => save(e.currentTarget.value)}
          onKeyDown={handleEnterBlur}
          class={styles.input}
        />
      </Match>

      <Match when={props.column.type === PropertyTypeDate}>
        <input type="date" value={value()} onChange={(e) => save(e.currentTarget.value)} class={styles.input} />
      </Match>

      <Match when={props.column.type === PropertyTypeURL}>
        <input
          type="url"
          value={value()}
          onBlur={(e) => save(e.currentTarget.value)}
          onKeyDown={handleEnterBlur}
          class={styles.input}
        />
      </Match>

      <Match when={props.column.type === PropertyTypeEmail}>
        <input
          type="email"
          value={value()}
          onBlur={(e) => save(e.currentTarget.value)}
          onKeyDown={handleEnterBlur}
          class={styles.input}
        />
      </Match>

      <Match when={props.column.type === PropertyTypePhone}>
        <input
          type="tel"
          value={value()}
          onBlur={(e) => save(e.currentTarget.value)}
          onKeyDown={handleEnterBlur}
          class={styles.input}
        />
      </Match>
    </Switch>
  );
}
