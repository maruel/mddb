// Grid view for table records, displaying data in cards.

import { For } from 'solid-js';
import type { DataRecordResponse, Property } from '../types.gen';
import styles from './TableGrid.module.css';
import { useI18n } from '../i18n';

interface TableGridProps {
  records: DataRecordResponse[];
  columns: Property[];
  onUpdateRecord?: (id: string, data: Record<string, unknown>) => void;
  onDeleteRecord: (id: string) => void;
}

export default function TableGrid(props: TableGridProps) {
  const { t } = useI18n();

  const handleUpdate = (record: DataRecordResponse, colName: string, value: string) => {
    if (record.data[colName] === value || !props.onUpdateRecord) return;
    const newData = { ...record.data, [colName]: value };
    props.onUpdateRecord(record.id, newData);
  };

  return (
    <div class={styles.grid}>
      <For each={props.records}>
        {(record) => (
          <div class={styles.card}>
            <div class={styles.cardHeader}>
              <strong>
                <input
                  type="text"
                  value={String((props.columns[0] ? record.data[props.columns[0].name] : null) || '')}
                  placeholder={t('table.untitled') || 'Untitled'}
                  onBlur={(e) => props.columns[0] && handleUpdate(record, props.columns[0].name, e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') e.currentTarget.blur();
                  }}
                  class={styles.titleInput}
                />
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
                    <input
                      type="text"
                      value={String(record.data[col.name] || '')}
                      onBlur={(e) => handleUpdate(record, col.name, e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') e.currentTarget.blur();
                      }}
                      class={styles.fieldValueInput}
                    />
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
