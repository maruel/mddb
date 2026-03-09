// Shared always-editable field input for card-style views (gallery, grid).

import { For, Show, Switch, Match, createSignal, createEffect, onCleanup, onMount } from 'solid-js';
import { Portal } from 'solid-js/web';
import {
  type DataRecordResponse,
  type Property,
  type SelectOption,
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
import styles from './FieldEditor.module.css';

const PRESET_COLORS = ['#e03e3e', '#e07b39', '#dfab01', '#4ca154', '#0b7285', '#1864ab', '#6741d9', '#c2255c'];

interface FieldEditorProps {
  record: DataRecordResponse;
  column: Property;
  onUpdate?: (id: string, data: Record<string, unknown>) => void;
  onUpdateOptions?: (options: SelectOption[]) => void;
}

interface DropPos {
  left: number;
  top: number;
  minWidth: number;
}

/** Compute viewport-relative position for a portal dropdown below a trigger element. */
function getDropPos(trigger: HTMLElement): DropPos {
  const rect = trigger.getBoundingClientRect();
  return { left: rect.left, top: rect.bottom + 4, minWidth: Math.max(rect.width, 160) };
}

export interface MultiSelectEditorProps {
  column: Property;
  value: string;
  onSave: (v: string) => void;
  /** Called when the editor requests to be closed (Escape or click-outside). */
  onClose?: () => void;
  /** Open the dropdown immediately on mount (used when embedded in a table cell). */
  autoOpen?: boolean;
  onUpdateOptions?: (options: SelectOption[]) => void;
}

export function MultiSelectEditor(props: MultiSelectEditorProps) {
  const [open, setOpen] = createSignal(false);
  const [dropPos, setDropPos] = createSignal<DropPos | null>(null);
  const [colorPickerFor, setColorPickerFor] = createSignal<string | null>(null);
  const [newTagName, setNewTagName] = createSignal('');
  let triggerRef: HTMLDivElement | undefined;
  let dropRef: HTMLDivElement | undefined;

  const openDropdown = () => {
    if (triggerRef) setDropPos(getDropPos(triggerRef));
    setOpen(true);
  };

  const closeDropdown = () => {
    setOpen(false);
    setDropPos(null);
    setColorPickerFor(null);
    setNewTagName('');
    props.onClose?.();
  };

  // Click-outside detection for the portal dropdown.
  createEffect(() => {
    if (!open()) return;
    const handler = (e: MouseEvent) => {
      const target = e.target as Node;
      if (!triggerRef?.contains(target) && !dropRef?.contains(target)) {
        setOpen(false);
        setDropPos(null);
        setColorPickerFor(null);
        setNewTagName('');
        props.onClose?.();
      }
    };
    const id = setTimeout(() => document.addEventListener('mousedown', handler), 0);
    onCleanup(() => {
      clearTimeout(id);
      document.removeEventListener('mousedown', handler);
    });
  });

  // Escape key.
  createEffect(() => {
    if (!open()) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        closeDropdown();
      }
    };
    document.addEventListener('keydown', handler, true);
    onCleanup(() => document.removeEventListener('keydown', handler, true));
  });

  onMount(() => {
    if (props.autoOpen) openDropdown();
  });

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

  const addNewOption = () => {
    const name = newTagName().trim();
    if (!name || !props.onUpdateOptions) return;
    const newOpt: SelectOption = { id: crypto.randomUUID(), name };
    const updated = [...(props.column.options ?? []), newOpt];
    props.onUpdateOptions(updated);
    // Auto-select the new option
    const current = selectedIds();
    props.onSave([...current, newOpt.id].join(','));
    setNewTagName('');
  };

  const changeOptionColor = (optId: string, color: string | undefined) => {
    if (!props.onUpdateOptions) return;
    const updated = (props.column.options ?? []).map((o) => (o.id === optId ? { ...o, color } : o));
    props.onUpdateOptions(updated);
    setColorPickerFor(null);
  };

  return (
    <div class={styles.multiSelectWrapper} ref={(el) => (triggerRef = el)} onClick={openDropdown}>
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
      <Show when={open() && dropPos()}>
        {(pos) => (
          <Portal>
            <div
              ref={(el) => (dropRef = el)}
              class={styles.portalDropdown}
              style={{
                left: `${pos().left}px`,
                top: `${pos().top}px`,
                'min-width': `${pos().minWidth}px`,
              }}
            >
              <For each={props.column.options ?? []}>
                {(opt) => (
                  <>
                    <Show when={colorPickerFor() === opt.id}>
                      <div class={styles.colorPicker}>
                        <For each={PRESET_COLORS}>
                          {(color) => (
                            <button
                              class={styles.colorSwatch}
                              style={{ background: color }}
                              type="button"
                              onMouseDown={(e) => {
                                e.preventDefault();
                                e.stopPropagation();
                                changeOptionColor(opt.id, color);
                              }}
                            />
                          )}
                        </For>
                        <button
                          class={`${styles.colorSwatch} ${styles.colorSwatchNone}`}
                          type="button"
                          onMouseDown={(e) => {
                            e.preventDefault();
                            e.stopPropagation();
                            changeOptionColor(opt.id, undefined);
                          }}
                        >
                          ×
                        </button>
                      </div>
                    </Show>
                    <div
                      class={`${styles.optionItem}${selectedIds().includes(opt.id) ? ` ${styles.optionSelected}` : ''}`}
                      onMouseDown={(e) => {
                        e.preventDefault();
                        toggle(opt.id);
                      }}
                    >
                      <button
                        class={styles.colorDot}
                        type="button"
                        style={opt.color ? { background: opt.color } : { background: 'var(--c-border)' }}
                        onClick={(e) => {
                          e.stopPropagation();
                          e.preventDefault();
                          setColorPickerFor(colorPickerFor() === opt.id ? null : opt.id);
                        }}
                      />
                      {opt.name}
                      <Show when={selectedIds().includes(opt.id)}>
                        <span class={styles.optionCheck}>✓</span>
                      </Show>
                    </div>
                  </>
                )}
              </For>
              <Show when={props.onUpdateOptions}>
                <div class={styles.addTagRow}>
                  <input
                    class={styles.addTagInput}
                    placeholder="Add option…"
                    value={newTagName()}
                    onInput={(e) => setNewTagName(e.currentTarget.value)}
                    onMouseDown={(e) => e.stopPropagation()}
                    onClick={(e) => e.stopPropagation()}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        addNewOption();
                      }
                    }}
                  />
                </div>
              </Show>
            </div>
          </Portal>
        )}
      </Show>
    </div>
  );
}

export interface SingleSelectEditorProps {
  column: Property;
  value: string;
  onSave: (v: string) => void;
  onClose?: () => void;
  autoOpen?: boolean;
  onUpdateOptions?: (options: SelectOption[]) => void;
}

export function SingleSelectEditor(props: SingleSelectEditorProps) {
  const [open, setOpen] = createSignal(false);
  const [dropPos, setDropPos] = createSignal<DropPos | null>(null);
  const [colorPickerFor, setColorPickerFor] = createSignal<string | null>(null);
  const [newTagName, setNewTagName] = createSignal('');
  let triggerRef: HTMLDivElement | undefined;
  let dropRef: HTMLDivElement | undefined;

  const openDropdown = () => {
    if (triggerRef) setDropPos(getDropPos(triggerRef));
    setOpen(true);
  };

  const closeDropdown = () => {
    setOpen(false);
    setDropPos(null);
    setColorPickerFor(null);
    setNewTagName('');
    props.onClose?.();
  };

  createEffect(() => {
    if (!open()) return;
    const handler = (e: MouseEvent) => {
      const target = e.target as Node;
      if (!triggerRef?.contains(target) && !dropRef?.contains(target)) {
        setOpen(false);
        setDropPos(null);
        setColorPickerFor(null);
        setNewTagName('');
        props.onClose?.();
      }
    };
    const id = setTimeout(() => document.addEventListener('mousedown', handler), 0);
    onCleanup(() => {
      clearTimeout(id);
      document.removeEventListener('mousedown', handler);
    });
  });

  createEffect(() => {
    if (!open()) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        closeDropdown();
      }
    };
    document.addEventListener('keydown', handler, true);
    onCleanup(() => document.removeEventListener('keydown', handler, true));
  });

  onMount(() => {
    if (props.autoOpen) openDropdown();
  });

  const selectedOption = () => props.column.options?.find((o) => o.id === props.value || o.name === props.value);

  const handleSelect = (optId: string) => {
    props.onSave(optId);
    closeDropdown();
  };

  const addNewOption = () => {
    const name = newTagName().trim();
    if (!name || !props.onUpdateOptions) return;
    const newOpt: SelectOption = { id: crypto.randomUUID(), name };
    const updated = [...(props.column.options ?? []), newOpt];
    props.onUpdateOptions(updated);
    // Single-select: just add the option, do NOT auto-select
    setNewTagName('');
  };

  const changeOptionColor = (optId: string, color: string | undefined) => {
    if (!props.onUpdateOptions) return;
    const updated = (props.column.options ?? []).map((o) => (o.id === optId ? { ...o, color } : o));
    props.onUpdateOptions(updated);
    setColorPickerFor(null);
  };

  return (
    <div
      class={styles.singleSelectWrapper}
      ref={(el) => (triggerRef = el)}
      onClick={() => (open() ? closeDropdown() : openDropdown())}
    >
      <Show when={selectedOption()} fallback={<span class={styles.selectPlaceholder}>--</span>}>
        {(opt) => (
          <span class={styles.chip} style={opt().color ? { background: opt().color, color: '#fff' } : {}}>
            {opt().name}
            <button
              class={styles.chipRemove}
              onClick={(e) => {
                e.stopPropagation();
                props.onSave('');
                closeDropdown();
              }}
              type="button"
              aria-label="Clear"
            >
              ×
            </button>
          </span>
        )}
      </Show>
      <Show when={open() && dropPos()}>
        {(pos) => (
          <Portal>
            <div
              ref={(el) => (dropRef = el)}
              class={styles.portalDropdown}
              style={{
                left: `${pos().left}px`,
                top: `${pos().top}px`,
                'min-width': `${pos().minWidth}px`,
              }}
            >
              <div
                class={styles.optionItem}
                onMouseDown={(e) => {
                  e.preventDefault();
                  props.onSave('');
                  closeDropdown();
                }}
              >
                <span class={styles.optionNone}>—</span>
              </div>
              <For each={props.column.options}>
                {(opt) => (
                  <>
                    <Show when={colorPickerFor() === opt.id}>
                      <div class={styles.colorPicker}>
                        <For each={PRESET_COLORS}>
                          {(color) => (
                            <button
                              class={styles.colorSwatch}
                              style={{ background: color }}
                              type="button"
                              onMouseDown={(e) => {
                                e.preventDefault();
                                e.stopPropagation();
                                changeOptionColor(opt.id, color);
                              }}
                            />
                          )}
                        </For>
                        <button
                          class={`${styles.colorSwatch} ${styles.colorSwatchNone}`}
                          type="button"
                          onMouseDown={(e) => {
                            e.preventDefault();
                            e.stopPropagation();
                            changeOptionColor(opt.id, undefined);
                          }}
                        >
                          ×
                        </button>
                      </div>
                    </Show>
                    <div
                      class={`${styles.optionItem}${selectedOption()?.id === opt.id ? ` ${styles.optionSelected}` : ''}`}
                      onMouseDown={(e) => {
                        e.preventDefault();
                        handleSelect(opt.id);
                      }}
                    >
                      <button
                        class={styles.colorDot}
                        type="button"
                        style={opt.color ? { background: opt.color } : { background: 'var(--c-border)' }}
                        onClick={(e) => {
                          e.stopPropagation();
                          e.preventDefault();
                          setColorPickerFor(colorPickerFor() === opt.id ? null : opt.id);
                        }}
                      />
                      {opt.name}
                      <Show when={selectedOption()?.id === opt.id}>
                        <span class={styles.optionCheck}>✓</span>
                      </Show>
                    </div>
                  </>
                )}
              </For>
              <Show when={props.onUpdateOptions}>
                <div class={styles.addTagRow}>
                  <input
                    class={styles.addTagInput}
                    placeholder="Add option…"
                    value={newTagName()}
                    onInput={(e) => setNewTagName(e.currentTarget.value)}
                    onMouseDown={(e) => e.stopPropagation()}
                    onClick={(e) => e.stopPropagation()}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        addNewOption();
                      }
                    }}
                  />
                </div>
              </Show>
            </div>
          </Portal>
        )}
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
        <SingleSelectEditor
          column={props.column}
          value={value()}
          onSave={save}
          onUpdateOptions={props.onUpdateOptions}
        />
      </Match>

      <Match when={props.column.type === PropertyTypeMultiSelect}>
        <MultiSelectEditor
          column={props.column}
          value={value()}
          onSave={save}
          onUpdateOptions={props.onUpdateOptions}
        />
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
