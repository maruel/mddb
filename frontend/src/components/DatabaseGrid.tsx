import { For } from 'solid-js';
import type { DataRecord, Property } from '../types';
import styles from './DatabaseGrid.module.css';
import { useI18n } from '../i18n';

interface DatabaseGridProps {
  records: DataRecord[];
  columns: Property[];
  onDeleteRecord: (id: string) => void;
}

export default function DatabaseGrid(props: DatabaseGridProps) {
  const { t } = useI18n();
  return (
    <div class={styles.grid}>
      <For each={props.records}>
        {(record) => (
          <div class={styles.card}>
            <div class={styles.cardHeader}>
              <strong>
                {String(
                  (props.columns[0] ? record.data[props.columns[0].name] : null) ||
                    t('database.untitled')
                )}
              </strong>
              <button class={styles.deleteBtn} onClick={() => props.onDeleteRecord(record.id)}>
                ✕
              </button>
            </div>
            <div class={styles.cardBody}>
              <For each={props.columns.slice(1, 4)}>
                {(col) => (
                  <div class={styles.field}>
                    <span class={styles.fieldName}>{col.name}:</span>
                    <span class={styles.fieldValue}>{String(record.data[col.name] || '-')}</span>
                  </div>
                )}
              </For>
            </div>
          </div>
        )}
      </For>
    </div>
  );
}
