// Shared resource quota form rendering the 6 ResourceQuotas fields for server, org, and workspace panels.

import { Show, For } from 'solid-js';
import { useI18n } from '../../i18n';
import type { ResourceQuotas } from '@sdk/types.gen';
import styles from './ResourceQuotaForm.module.css';

interface ResourceQuotaFormProps {
  value: () => ResourceQuotas;
  onChange: (v: ResourceQuotas) => void;
  // When provided, shows a ceiling hint below each field.
  ceiling?: () => ResourceQuotas | null;
  ceilingLabel?: string;
  // When true, shows an "Inherit from parent" checkbox; value of -1 means inherit.
  allowInherit?: boolean;
}

type QuotaKey = keyof ResourceQuotas;

export default function ResourceQuotaForm(props: ResourceQuotaFormProps) {
  const { t } = useI18n();

  const update = (key: QuotaKey, val: number) => {
    props.onChange({ ...props.value(), [key]: val });
  };

  const toggleInherit = (key: QuotaKey) => {
    const cur = (props.value()[key] as number) ?? 0;
    if (cur === -1) {
      const ceil = props.ceiling?.()?.[key] as number | undefined;
      update(key, ceil ?? 0);
    } else {
      update(key, -1);
    }
  };

  const fields: Array<{ key: QuotaKey; label: string }> = [
    { key: 'max_pages', label: t('settings.maxPages') },
    { key: 'max_storage_bytes', label: t('settings.maxStorageBytes') },
    { key: 'max_records_per_table', label: t('settings.maxRecordsPerTable') },
    { key: 'max_asset_size_bytes', label: t('settings.maxAssetSizeBytes') },
    { key: 'max_tables_per_workspace', label: t('settings.maxTablesPerWorkspace') },
    { key: 'max_columns_per_table', label: t('settings.maxColumnsPerTable') },
  ];

  return (
    <div class={styles.formGrid}>
      <For each={fields}>
        {(field) => {
          const val = () => (props.value()[field.key] as number) ?? 0;
          const ceilVal = () => props.ceiling?.()?.[field.key] as number | undefined;
          const isInherited = () => val() === -1;
          const ceilDisplay = () => {
            const v = ceilVal();
            return v !== undefined && v > 0 ? v : null;
          };

          return (
            <div class={styles.formItem}>
              <label>{field.label}</label>
              <Show when={props.allowInherit}>
                <label class={styles.inheritLabel}>
                  <input type="checkbox" checked={isInherited()} onChange={() => toggleInherit(field.key)} />
                  {t('settings.inheritFromParent')}
                </label>
              </Show>
              <Show
                when={!isInherited()}
                fallback={<input type="number" value={ceilDisplay() ?? ''} disabled class={styles.inheritedInput} />}
              >
                <input
                  type="number"
                  value={val()}
                  onInput={(e) => update(field.key, parseInt(e.target.value) || 0)}
                  min="0"
                />
                <Show when={ceilDisplay()}>
                  {(v) => (
                    <p class={styles.ceilingHint}>
                      {props.ceilingLabel ?? t('settings.parentCeiling')}: {v()}
                    </p>
                  )}
                </Show>
              </Show>
            </div>
          );
        }}
      </For>
    </div>
  );
}
