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
  PropertyTypeUser,
} from '@sdk/types.gen';
import { useRecords } from '../../contexts/RecordsContext';
import { useI18n } from '../../i18n';
import { chipTextColor } from './tableUtils';
import styles from './FieldValue.module.css';

function truncateUrl(url: string): string {
  try {
    const { hostname, pathname } = new URL(url);
    if (pathname === '/') return hostname;
    const full = hostname + pathname;
    if (full.length <= 32) return full;
    const start = 4;
    const end = 7;
    return hostname + pathname.slice(0, start + 1) + '\u2026' + pathname.slice(-end);
  } catch {
    return url.length > 32 ? url.slice(0, 29) + '\u2026' : url;
  }
}

interface FieldValueProps {
  record: DataRecordResponse;
  column: Property;
}

/**
 * Renders a record field value in read mode with type-appropriate formatting.
 * Used by table, gallery, and grid views.
 */
export function FieldValue(props: FieldValueProps): JSXElement {
  const { t } = useI18n();
  // resolvedUsers may be undefined if used outside RecordsProvider (e.g. tests).
  let records: ReturnType<typeof useRecords> | undefined;
  try {
    records = useRecords();
  } catch {
    // Outside RecordsProvider context — user columns will fall back to showing the raw ID.
  }

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
      <span class={styles.selectChip} style={color ? { background: color, color: chipTextColor(color) } : {}}>
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
            {truncateUrl(strValue())}
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
      <Match when={props.column.type === PropertyTypeUser}>
        <Show when={strValue()}>
          {(() => {
            const userId = strValue();
            const resolved = records?.resolvedUsers().get(userId);
            const name = resolved?.name ?? userId;
            const avatarUrl = resolved?.avatar_url;
            const isGhost = resolved?.is_ghost ?? false;
            const initials = name
              .split(' ')
              .map((w) => w[0])
              .join('')
              .slice(0, 2)
              .toUpperCase();
            return (
              <span class={`${styles.userChip}${isGhost ? ` ${styles.ghost}` : ''}`}>
                <Show when={avatarUrl} fallback={<span class={styles.userAvatar}>{initials}</span>}>
                  <img src={avatarUrl} class={styles.userAvatar} alt="" />
                </Show>
                {name}
                <Show when={isGhost}>
                  <span class={styles.userGhostLabel}>({t('table.userRemoved')})</span>
                </Show>
              </span>
            );
          })()}
        </Show>
      </Match>
    </Switch>
  );
}
