// Shared always-editable field input for card-style views (gallery, grid).

import { For, Switch, Match } from 'solid-js';
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
import styles from './FieldEditor.module.css';

interface FieldEditorProps {
  record: DataRecordResponse;
  column: Property;
  onUpdate?: (id: string, data: Record<string, unknown>) => void;
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
        <input
          type="text"
          value={value()}
          onBlur={(e) => save(e.currentTarget.value)}
          onKeyDown={handleEnterBlur}
          class={styles.input}
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
