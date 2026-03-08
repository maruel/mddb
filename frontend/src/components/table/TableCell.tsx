// Table cell component with inline editing for different property types.

import { createSignal, Show, For, type Accessor, type JSXElement } from 'solid-js';
import {
  type DataRecordResponse,
  type Property,
  PropertyTypeCheckbox,
  PropertyTypeSelect,
  PropertyTypeMultiSelect,
  PropertyTypeNumber,
  PropertyTypeDate,
} from '@sdk/types.gen';
import styles from './TableCell.module.css';

export interface TableCellProps {
  record: DataRecordResponse;
  column: Property;
  isEditing: Accessor<boolean>;
  onStartEdit: () => void;
  onSave: (value: string) => void;
  onCancel: () => void;
  onTabNext?: () => void;
  onTabPrev?: () => void;
  onEnterDown?: () => void;
}

/**
 * Table cell with inline editing support.
 * Renders appropriate input based on column type when editing.
 */
export function TableCell(props: TableCellProps) {
  const [editValue, setEditValue] = createSignal('');
  const [editCancelled, setEditCancelled] = createSignal(false);
  let inputRef: HTMLInputElement | HTMLSelectElement | undefined;

  const getCellValue = () => {
    return props.record.data[props.column.name] ?? '';
  };

  const handleClick = () => {
    if (!props.isEditing()) {
      setEditValue(String(getCellValue()));
      setEditCancelled(false);
      props.onStartEdit();
    }
  };

  const renderSelectChip = (id: string): JSXElement => {
    const opt = props.column.options?.find((o) => o.id === id || o.name === id);
    const label = opt?.name ?? id;
    const color = opt?.color;
    return (
      <span class={styles.selectChip} style={color ? { background: color, color: '#fff' } : {}}>
        {label}
      </span>
    );
  };

  const renderCellContent = (): JSXElement => {
    const value = getCellValue();

    switch (props.column.type) {
      case 'checkbox':
        return <span class={styles.checkbox}>{value === 'true' || value === true ? '✓' : ''}</span>;
      case PropertyTypeSelect: {
        const id = String(value);
        return id ? renderSelectChip(id) : null;
      }
      case PropertyTypeMultiSelect: {
        const ids = String(value)
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean);
        return ids.length > 0 ? (
          <span class={styles.multiChips}>
            <For each={ids}>{(id) => renderSelectChip(id)}</For>
          </span>
        ) : null;
      }
      case 'date':
        return value ? new Date(value as string).toLocaleDateString() : '';
      default:
        return String(value);
    }
  };

  // Save on blur or Enter, cancel on Escape
  const handleCellBlur = () => {
    if (editCancelled()) {
      setEditCancelled(false);
      return;
    }
    handleCellSave();
  };

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleCellSave();
      props.onEnterDown?.();
    } else if (e.key === 'Escape') {
      e.preventDefault();
      setEditCancelled(true);
      props.onCancel();
    } else if (e.key === 'Tab') {
      e.preventDefault();
      handleCellSave();
      if (e.shiftKey) {
        props.onTabPrev?.();
      } else {
        props.onTabNext?.();
      }
    }
  };

  const handleCellSave = () => {
    if (editCancelled() || !props.isEditing()) {
      return;
    }
    props.onSave(editValue());
  };

  // Read current value from input ref for saving
  const getCurrentValue = () => {
    if (!inputRef) return editValue();
    if (inputRef instanceof HTMLInputElement && inputRef.type === 'checkbox') {
      return String(inputRef.checked);
    }
    return inputRef.value;
  };

  const syncAndSave = () => {
    setEditValue(getCurrentValue());
    handleCellSave();
  };

  const syncAndBlur = () => {
    setEditValue(getCurrentValue());
    handleCellBlur();
  };

  const onKeyDown = (e: KeyboardEvent) => {
    // Sync current value from input before handling key press
    if (e.key === 'Enter' || e.key === 'Tab' || e.key === 'Escape') {
      setEditValue(getCurrentValue());
    }
    handleKeyDown(e);
  };

  const focusRef = (el: HTMLInputElement | HTMLSelectElement) => {
    inputRef = el;
    setTimeout(() => {
      el.focus();
      if (el instanceof HTMLInputElement && el.type === 'text') el.select();
    }, 0);
  };

  const renderCellInput = () => {
    const initialValue = editValue();

    switch (props.column.type) {
      case PropertyTypeCheckbox:
        return (
          <input
            ref={focusRef}
            type="checkbox"
            checked={initialValue === 'true'}
            onChange={syncAndSave}
            onKeyDown={onKeyDown}
            class={styles.input}
          />
        );
      case PropertyTypeSelect:
        if (props.column.options && props.column.options.length > 0) {
          return (
            <select
              ref={focusRef}
              value={initialValue}
              onChange={syncAndSave}
              onBlur={syncAndBlur}
              onKeyDown={onKeyDown}
              class={styles.input}
            >
              <option value="">--</option>
              <For each={props.column.options}>{(option) => <option value={option.id}>{option.name}</option>}</For>
            </select>
          );
        }
        return (
          <input
            ref={focusRef}
            type="text"
            value={initialValue}
            onBlur={syncAndBlur}
            onKeyDown={onKeyDown}
            class={styles.input}
          />
        );
      case PropertyTypeNumber:
        return (
          <input
            ref={focusRef}
            type="number"
            value={initialValue}
            onBlur={syncAndBlur}
            onKeyDown={onKeyDown}
            class={styles.input}
          />
        );
      case PropertyTypeDate:
        return (
          <input
            ref={focusRef}
            type="date"
            value={initialValue}
            onBlur={syncAndBlur}
            onKeyDown={onKeyDown}
            class={styles.input}
          />
        );
      default:
        return (
          <input
            ref={focusRef}
            type="text"
            value={initialValue}
            onBlur={syncAndBlur}
            onKeyDown={onKeyDown}
            class={styles.input}
          />
        );
    }
  };

  return (
    <td class={`${styles.cell}${props.isEditing() ? ` ${styles.editing}` : ''}`} onClick={handleClick}>
      <Show when={props.isEditing()} fallback={<div class={styles.cellContent}>{renderCellContent()}</div>}>
        {renderCellInput()}
      </Show>
    </td>
  );
}
