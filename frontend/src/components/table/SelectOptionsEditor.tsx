// Option management panel for select and multi_select columns.

import { createSignal, For, Show, createEffect, onCleanup, untrack } from 'solid-js';
import { Portal } from 'solid-js/web';
import type { Property, SelectOption } from '@sdk/types.gen';
import { useI18n } from '../../i18n';
import styles from './SelectOptionsEditor.module.css';

import DeleteIcon from '@material-symbols/svg-400/outlined/delete.svg?solid';
import CloseIcon from '@material-symbols/svg-400/outlined/close.svg?solid';
import DragIndicatorIcon from '@material-symbols/svg-400/outlined/drag_indicator.svg?solid';

/** 12 accessible preset swatches. '#ffffff' = no color (renders as default). */
export const OPTION_COLORS = [
  '#e03e3e',
  '#d9730d',
  '#dfab01',
  '#0f7b6c',
  '#0b6e99',
  '#6940a5',
  '#ad1a72',
  '#64473a',
  '#9b9a97',
  '#37352f',
  '#787774',
  '#ffffff',
];

function generateOptionId(existing: SelectOption[]): string {
  const ids = new Set(existing.map((o) => o.id));
  let id: string;
  do {
    id = crypto.randomUUID().slice(0, 8);
  } while (ids.has(id));
  return id;
}

interface SelectOptionsEditorProps {
  column: Property;
  allColumns: Property[];
  records: { data: Record<string, unknown> }[];
  position: { x: number; y: number };
  onUpdateColumns: (cols: Property[]) => Promise<void>;
  onClose: () => void;
}

/**
 * Portal panel for managing select/multi_select column options.
 * Opened via the column header context menu "Edit options" action.
 */
export function SelectOptionsEditor(props: SelectOptionsEditorProps) {
  const { t } = useI18n();
  let panelRef: HTMLDivElement | undefined;

  const [localOptions, setLocalOptions] = createSignal<SelectOption[]>(
    untrack(() => (props.column.options ?? []).map((o) => ({ ...o })))
  );
  const [openSwatchFor, setOpenSwatchFor] = createSignal<string | null>(null);
  const [pendingSave, setPendingSave] = createSignal(false);

  // Viewport boundary clamping
  const [adjustedPos, setAdjustedPos] = createSignal(untrack(() => ({ x: props.position.x, y: props.position.y })));
  createEffect(() => {
    if (!panelRef) return;
    const rect = panelRef.getBoundingClientRect();
    const padding = 8;
    let x = props.position.x;
    let y = props.position.y;
    if (x + rect.width > window.innerWidth - padding) x = Math.max(padding, window.innerWidth - rect.width - padding);
    if (y + rect.height > window.innerHeight - padding)
      y = Math.max(padding, window.innerHeight - rect.height - padding);
    setAdjustedPos({ x, y });
  });

  // Click-outside to close
  createEffect(() => {
    const handler = (e: MouseEvent) => {
      if (panelRef && !panelRef.contains(e.target as Node)) {
        props.onClose();
      }
    };
    const id = setTimeout(() => document.addEventListener('mousedown', handler), 0);
    onCleanup(() => {
      clearTimeout(id);
      document.removeEventListener('mousedown', handler);
    });
  });

  // Escape to close
  createEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        props.onClose();
      }
    };
    document.addEventListener('keydown', handler);
    onCleanup(() => document.removeEventListener('keydown', handler));
  });

  const save = async (opts: SelectOption[]) => {
    const updated = props.allColumns.map((col) => (col.name === props.column.name ? { ...col, options: opts } : col));
    setPendingSave(true);
    try {
      await props.onUpdateColumns(updated);
    } finally {
      setPendingSave(false);
    }
  };

  const handleRename = (id: string, name: string) => {
    const next = localOptions().map((o) => (o.id === id ? { ...o, name } : o));
    setLocalOptions(next);
  };

  const handleRenameBlur = (id: string, name: string) => {
    const trimmed = name.trim();
    if (!trimmed) return;
    const next = localOptions().map((o) => (o.id === id ? { ...o, name: trimmed } : o));
    setLocalOptions(next);
    void save(next);
  };

  const handleRecolor = (id: string, color: string) => {
    const c = color === '#ffffff' ? undefined : color;
    const next = localOptions().map((o) => (o.id === id ? { ...o, color: c } : o));
    setLocalOptions(next);
    setOpenSwatchFor(null);
    void save(next);
  };

  const handleDelete = (id: string) => {
    const next = localOptions().filter((o) => o.id !== id);
    setLocalOptions(next);
    void save(next);
  };

  const handleAddOption = () => {
    const newOpt: SelectOption = { id: generateOptionId(localOptions()), name: '' };
    const next = [...localOptions(), newOpt];
    setLocalOptions(next);
    // Focus the new input on next tick
    setTimeout(() => {
      const inputs = panelRef?.querySelectorAll<HTMLInputElement>('.' + styles.optionNameInput);
      inputs?.[inputs.length - 1]?.focus();
    }, 0);
  };

  const usageCount = (id: string): number => {
    return props.records.filter((r) => {
      const v = String(r.data[props.column.name] ?? '');
      return v
        .split(',')
        .map((s) => s.trim())
        .includes(id);
    }).length;
  };

  // Drag-to-reorder state
  const [draggingId, setDraggingId] = createSignal<string | null>(null);
  const [dragOverId, setDragOverId] = createSignal<string | null>(null);

  const handleDragStart = (id: string) => setDraggingId(id);
  const handleDragOver = (e: DragEvent, id: string) => {
    e.preventDefault();
    if (id !== draggingId()) setDragOverId(id);
  };
  const handleDrop = (targetId: string) => {
    const srcId = draggingId();
    setDraggingId(null);
    setDragOverId(null);
    if (!srcId || srcId === targetId) return;
    const opts = [...localOptions()];
    const srcIdx = opts.findIndex((o) => o.id === srcId);
    if (srcIdx < 0) return;
    const [moved] = opts.splice(srcIdx, 1) as [SelectOption];
    const targetIdx = opts.findIndex((o) => o.id === targetId);
    if (targetIdx < 0) return;
    opts.splice(targetIdx, 0, moved);
    setLocalOptions(opts);
    void save(opts);
  };
  const handleDragEnd = () => {
    setDraggingId(null);
    setDragOverId(null);
  };

  return (
    <Portal>
      <div
        ref={(el) => (panelRef = el)}
        class={styles.panel}
        style={{ left: `${adjustedPos().x}px`, top: `${adjustedPos().y}px` }}
        data-testid="select-options-editor"
      >
        <div class={styles.header}>
          <span class={styles.title}>{props.column.name}</span>
          <button class={styles.closeBtn} onClick={() => props.onClose()} aria-label={t('common.close') || 'Close'}>
            <CloseIcon />
          </button>
        </div>

        <div class={styles.optionList}>
          <For each={localOptions()}>
            {(opt) => {
              const count = usageCount(opt.id);
              const isSwatchOpen = () => openSwatchFor() === opt.id;
              const isDragging = () => draggingId() === opt.id;
              const isDragOver = () => dragOverId() === opt.id;

              return (
                <div
                  class={styles.optionRow}
                  classList={{
                    [`${styles.optionRowDragging}`]: isDragging(),
                    [`${styles.optionRowDragOver}`]: isDragOver(),
                  }}
                  draggable={true}
                  onDragStart={() => handleDragStart(opt.id)}
                  onDragOver={(e) => handleDragOver(e, opt.id)}
                  onDrop={() => handleDrop(opt.id)}
                  onDragEnd={handleDragEnd}
                  data-testid={`option-row-${opt.id}`}
                >
                  <span class={styles.dragHandle} title="Drag to reorder">
                    <DragIndicatorIcon />
                  </span>

                  {/* Color swatch */}
                  <div class={styles.swatchWrapper}>
                    <button
                      class={styles.swatchBtn}
                      style={opt.color ? { background: opt.color } : {}}
                      onClick={() => setOpenSwatchFor(isSwatchOpen() ? null : opt.id)}
                      aria-label="Change color"
                      data-testid={`option-color-${opt.id}`}
                    />
                    <Show when={isSwatchOpen()}>
                      <div class={styles.swatchPicker} data-testid="swatch-picker">
                        <For each={OPTION_COLORS}>
                          {(color) => (
                            <button
                              class={styles.swatchChoice}
                              style={
                                color === '#ffffff'
                                  ? { background: 'var(--c-bg-hover)', border: '1px solid var(--c-border)' }
                                  : { background: color }
                              }
                              classList={{ [`${styles.swatchChoiceActive}`]: (opt.color ?? '') === color }}
                              onClick={() => handleRecolor(opt.id, color)}
                              aria-label={color}
                              data-testid={`swatch-${color}`}
                            />
                          )}
                        </For>
                      </div>
                    </Show>
                  </div>

                  <input
                    type="text"
                    class={styles.optionNameInput}
                    value={opt.name}
                    placeholder={t('table.optionPlaceholder') || 'Option name'}
                    onInput={(e) => handleRename(opt.id, e.currentTarget.value)}
                    onBlur={(e) => handleRenameBlur(opt.id, e.currentTarget.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') e.currentTarget.blur();
                    }}
                    data-testid={`option-name-${opt.id}`}
                  />

                  <Show when={count > 0}>
                    <span class={styles.usageHint} title={t('table.optionUsedWarning').replace('{n}', String(count))}>
                      {count}
                    </span>
                  </Show>

                  <button
                    class={styles.deleteBtn}
                    onClick={() => handleDelete(opt.id)}
                    aria-label={t('table.deleteOption') || 'Delete option'}
                    data-testid={`option-delete-${opt.id}`}
                  >
                    <DeleteIcon />
                  </button>
                </div>
              );
            }}
          </For>
        </div>

        <button
          class={styles.addOptionBtn}
          onClick={handleAddOption}
          disabled={pendingSave()}
          data-testid="add-option-btn"
        >
          + {t('table.addOption') || 'Add an option'}
        </button>
      </div>
    </Portal>
  );
}
