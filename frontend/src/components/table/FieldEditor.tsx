// Shared always-editable field input for card-style views (gallery, grid).

import { For, Show, Switch, Match, createSignal, createEffect, onCleanup, onMount } from 'solid-js';
import { Portal } from 'solid-js/web';
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
  PropertyTypeUser,
} from '@sdk/types.gen';
import { updateRecordField, handleEnterBlur, getFieldValue } from './tableUtils';
import { useRecords } from '../../contexts/RecordsContext';
import { useI18n } from '../../i18n';
import styles from './FieldEditor.module.css';

interface FieldEditorProps {
  record: DataRecordResponse;
  column: Property;
  onUpdate?: (id: string, data: Record<string, unknown>) => void;
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
}

export function MultiSelectEditor(props: MultiSelectEditorProps) {
  const [open, setOpen] = createSignal(false);
  const [dropPos, setDropPos] = createSignal<DropPos | null>(null);
  let triggerRef: HTMLDivElement | undefined;
  let dropRef: HTMLDivElement | undefined;

  const openDropdown = () => {
    if (triggerRef) setDropPos(getDropPos(triggerRef));
    setOpen(true);
  };

  const closeDropdown = () => {
    setOpen(false);
    setDropPos(null);
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

  const unselectedOptions = () => (props.column.options ?? []).filter((o) => !selectedIds().includes(o.id));

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
      <Show when={open() && unselectedOptions().length > 0 && dropPos()}>
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
              <For each={unselectedOptions()}>
                {(opt) => (
                  <div
                    class={styles.optionItem}
                    onMouseDown={(e) => {
                      e.preventDefault(); // prevent blur before toggle
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
}

export function SingleSelectEditor(props: SingleSelectEditorProps) {
  const [open, setOpen] = createSignal(false);
  const [dropPos, setDropPos] = createSignal<DropPos | null>(null);
  let triggerRef: HTMLDivElement | undefined;
  let dropRef: HTMLDivElement | undefined;

  const openDropdown = () => {
    if (triggerRef) setDropPos(getDropPos(triggerRef));
    setOpen(true);
  };

  const closeDropdown = () => {
    setOpen(false);
    setDropPos(null);
    props.onClose?.();
  };

  createEffect(() => {
    if (!open()) return;
    const handler = (e: MouseEvent) => {
      const target = e.target as Node;
      if (!triggerRef?.contains(target) && !dropRef?.contains(target)) {
        setOpen(false);
        setDropPos(null);
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

  const handleClear = (e: MouseEvent) => {
    e.stopPropagation();
    props.onSave('');
    closeDropdown();
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
            <button class={styles.chipRemove} onClick={handleClear} type="button" aria-label="Clear">
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
                  <div
                    class={styles.optionItem}
                    onMouseDown={(e) => {
                      e.preventDefault();
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
          </Portal>
        )}
      </Show>
    </div>
  );
}

export interface UserEditorProps {
  value: string;
  onSave: (v: string) => void;
  onClose?: () => void;
  autoOpen?: boolean;
}

/** Dropdown editor for user-type columns. Shows workspace members as options. */
export function UserEditor(props: UserEditorProps) {
  const { t } = useI18n();
  const [open, setOpen] = createSignal(false);
  const [dropPos, setDropPos] = createSignal<DropPos | null>(null);
  let triggerRef: HTMLDivElement | undefined;
  let dropRef: HTMLDivElement | undefined;

  let records: ReturnType<typeof useRecords> | undefined;
  try {
    records = useRecords();
  } catch {
    // Outside provider — no members available.
  }

  const members = () => records?.workspaceMembers() ?? [];
  const resolvedUsers = () => records?.resolvedUsers() ?? new Map();

  const openDropdown = () => {
    if (triggerRef) setDropPos(getDropPos(triggerRef));
    setOpen(true);
  };

  const closeDropdown = () => {
    setOpen(false);
    setDropPos(null);
    props.onClose?.();
  };

  createEffect(() => {
    if (!open()) return;
    const handler = (e: MouseEvent) => {
      const target = e.target as Node;
      if (!triggerRef?.contains(target) && !dropRef?.contains(target)) {
        setOpen(false);
        setDropPos(null);
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

  const selectedMember = () => {
    const id = props.value;
    if (!id) return null;
    const resolved = resolvedUsers().get(id);
    if (resolved) return resolved;
    const member = members().find((m) => m.id === id);
    if (member)
      return { id: member.id, name: member.name, email: member.email, avatar_url: member.avatar_url, is_ghost: false };
    return null;
  };

  const handleSelect = (userId: string) => {
    props.onSave(userId);
    closeDropdown();
  };

  const handleClear = (e: MouseEvent) => {
    e.stopPropagation();
    props.onSave('');
    closeDropdown();
  };

  return (
    <div
      class={styles.singleSelectWrapper}
      ref={(el) => (triggerRef = el)}
      onClick={() => (open() ? closeDropdown() : openDropdown())}
    >
      <Show when={selectedMember()} fallback={<span class={styles.selectPlaceholder}>--</span>}>
        {(user) => {
          const initials = user()
            .name.split(' ')
            .map((w: string) => w[0])
            .join('')
            .slice(0, 2)
            .toUpperCase();
          return (
            <span class={styles.chip}>
              <Show when={user().avatar_url} fallback={<span class={styles.memberAvatar}>{initials}</span>}>
                <img src={user().avatar_url} class={styles.memberAvatar} alt="" />
              </Show>
              {user().name}
              <Show when={user().is_ghost}>
                {' '}
                <span class={styles.ghostLabel}>({t('table.userRemoved')})</span>
              </Show>
              <button class={styles.chipRemove} onClick={handleClear} type="button" aria-label="Clear">
                ×
              </button>
            </span>
          );
        }}
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
              <For each={members()}>
                {(member) => {
                  const initials = member.name
                    .split(' ')
                    .map((w) => w[0])
                    .join('')
                    .slice(0, 2)
                    .toUpperCase();
                  return (
                    <div
                      class={styles.optionItem}
                      onMouseDown={(e) => {
                        e.preventDefault();
                        handleSelect(member.id);
                      }}
                    >
                      <Show when={member.avatar_url} fallback={<span class={styles.memberAvatar}>{initials}</span>}>
                        <img src={member.avatar_url} class={styles.memberAvatar} alt="" />
                      </Show>
                      {member.name}
                      <span class={styles.memberEmail}>{member.email}</span>
                    </div>
                  );
                }}
              </For>
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

      <Match when={props.column.type === PropertyTypeUser}>
        <UserEditor value={value()} onSave={save} />
      </Match>
    </Switch>
  );
}
