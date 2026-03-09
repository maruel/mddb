// Shared always-editable field input for card-style views (gallery, grid).

import { For, Show, Switch, Match, createSignal } from 'solid-js';
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

interface MultiSelectEditorProps {
  column: Property;
  value: string;
  onSave: (v: string) => void;
  /** Called when the editor requests to be closed (Escape or click-outside). */
  onClose?: () => void;
}

export function MultiSelectEditor(props: MultiSelectEditorProps) {
  const [open, setOpen] = createSignal(false);
  let wrapperRef: HTMLDivElement | undefined;

  useClickOutside(
    () => wrapperRef,
    () => {
      setOpen(false);
      props.onClose?.();
    }
  );

  const selectedIds = () =>
    props.value
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);

  const optionByIdOrName = (id: string) => props.column.options?.find((o) => o.id === id || o.name === id);

  const optionName = (id: string) => optionByIdOrName(id)?.name ?? id;
  const optionColor = (id: string) => optionByIdOrName(id)?.color;

  const toggle = (id: string) => {
    const current = selectedIds();
    const next = current.includes(id) ? current.filter((x) => x !== id) : [...current, id];
    props.onSave(next.join(','));
  };

  const unselectedOptions = () => (props.column.options ?? []).filter((o) => !selectedIds().includes(o.id));

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      setOpen(false);
      props.onClose?.();
    }
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
          {(id) => {
            const color = optionColor(id);
            return (
              <span class={styles.chip} style={color ? { background: color, color: '#fff' } : {}}>
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
            );
          }}
        </For>
      </div>
      <Show when={open() && unselectedOptions().length > 0}>
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
                <Show when={opt.color}>
                  <span class={styles.optionColor} style={{ background: opt.color }} />
                </Show>
                {opt.name}
              </div>
            )}
          </For>
        </div>
      </Show>
    </div>
  );
}

interface SingleSelectEditorProps {
  column: Property;
  value: string;
  onSave: (v: string) => void;
  onClose?: () => void;
}

function SingleSelectEditor(props: SingleSelectEditorProps) {
  const [open, setOpen] = createSignal(false);
  let wrapperRef: HTMLDivElement | undefined;

  useClickOutside(
    () => wrapperRef,
    () => {
      setOpen(false);
      props.onClose?.();
    }
  );

  const selectedOption = () => props.column.options?.find((o) => o.id === props.value || o.name === props.value);

  const handleSelect = (optId: string) => {
    props.onSave(optId);
    setOpen(false);
    props.onClose?.();
  };

  const handleClear = (e: MouseEvent) => {
    e.stopPropagation();
    props.onSave('');
    setOpen(false);
    props.onClose?.();
  };

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      setOpen(false);
      props.onClose?.();
    }
  };

  return (
    <div
      class={styles.singleSelectWrapper}
      ref={(el) => (wrapperRef = el)}
      onKeyDown={handleKeyDown}
      onClick={() => setOpen(!open())}
    >
      <Show when={selectedOption()} fallback={<span class={styles.selectPlaceholder}>--</span>}>
        {(opt) => (
          <span class={styles.chip} style={opt().color ? { background: opt().color, color: '#fff' } : {}}>
            {opt().name}
            <button class={styles.chipRemove} onClick={handleClear} type="button" aria-label="Clear">
              ×
            </button>
          </span>
        )}
      </Show>
      <Show when={open()}>
        <div class={styles.optionsList}>
          <div
            class={styles.optionItem}
            onClick={(e) => {
              e.stopPropagation();
              props.onSave('');
              setOpen(false);
              props.onClose?.();
            }}
          >
            <span class={styles.optionNone}>—</span>
          </div>
          <For each={props.column.options}>
            {(opt) => (
              <div
                class={styles.optionItem}
                onClick={(e) => {
                  e.stopPropagation();
                  handleSelect(opt.id);
                }}
              >
                <Show when={opt.color}>
                  <span class={styles.optionColor} style={{ background: opt.color }} />
                </Show>
                {opt.name}
              </div>
            )}
          </For>
        </div>
      </Show>
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
        <SingleSelectEditor column={props.column} value={value()} onSave={save} />
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
