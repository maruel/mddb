// Shared read-mode field renderer for all property types.

import { For, Show, Switch, Match } from 'solid-js';
import type { JSXElement } from 'solid-js';
import type { DataRecordResponse, Property } from '@sdk/types.gen';
import {
  PropertyTypeCheckbox,
  PropertyTypeSelect,
  PropertyTypeMultiSelect,
  PropertyTypeDate,
  PropertyTypeURL,
  PropertyTypeEmail,
  PropertyTypePhone,
} from '@sdk/types.gen';
import styles from './FieldValue.module.css';

interface FieldValueProps {
  record: DataRecordResponse;
  column: Property;
}

/**
 * Renders a record field value in read mode with type-appropriate formatting.
 * Used by table, gallery, and grid views.
 */
export function FieldValue(props: FieldValueProps): JSXElement {
  const rawValue = () => props.record.data[props.column.name] ?? '';
  const strValue = () => String(rawValue());
  const chips = () =>
    strValue()
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);

  const renderChip = (id: string): JSXElement => {
    const opt = props.column.options?.find((o) => o.id === id || o.name === id);
    const label = opt?.name ?? id;
    const color = opt?.color;
    return (
      <span class={styles.selectChip} style={color ? { background: color, color: '#fff' } : {}}>
        {label}
      </span>
    );
  };

  return (
    <Switch fallback={<>{strValue()}</>}>
      <Match when={props.column.type === PropertyTypeCheckbox}>
        <input
          type="checkbox"
          class={styles.checkbox}
          checked={rawValue() === 'true' || rawValue() === true}
          readOnly
        />
      </Match>
      <Match when={props.column.type === PropertyTypeSelect}>
        <Show when={strValue()}>{renderChip(strValue())}</Show>
      </Match>
      <Match when={props.column.type === PropertyTypeMultiSelect}>
        <Show when={chips().length > 0}>
          <span class={styles.multiChips}>
            <For each={chips()}>{(id) => renderChip(id)}</For>
          </span>
        </Show>
      </Match>
      <Match when={props.column.type === PropertyTypeDate}>
        <Show when={rawValue()}>{new Date(strValue()).toLocaleDateString()}</Show>
      </Match>
      <Match when={props.column.type === PropertyTypeURL}>
        <Show when={strValue()}>
          <a href={strValue()} class={styles.link} target="_blank" rel="noopener noreferrer">
            {strValue()}
          </a>
        </Show>
      </Match>
      <Match when={props.column.type === PropertyTypeEmail}>
        <Show when={strValue()}>
          <a href={`mailto:${strValue()}`} class={styles.link}>
            {strValue()}
          </a>
        </Show>
      </Match>
      <Match when={props.column.type === PropertyTypePhone}>
        <Show when={strValue()}>
          <a href={`tel:${strValue()}`} class={styles.link}>
            {strValue()}
          </a>
        </Show>
      </Match>
    </Switch>
  );
}
