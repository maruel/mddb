// Grid view for table records, displaying data in cards.

import { For } from 'solid-js';
import type { DataRecordResponse, Property } from '@sdk/types.gen';
import { updateRecordField, handleEnterBlur, getRecordTitle, getFieldValue } from './table/tableUtils';
import { useI18n } from '../i18n';
import styles from './TableGrid.module.css';

interface TableGridProps {
  records: DataRecordResponse[];
  columns: Property[];
  onUpdateRecord?: (id: string, data: Record<string, unknown>) => void;
  onDeleteRecord: (id: string) => void;
}

export default function TableGrid(props: TableGridProps) {
  const { t } = useI18n();

  return (
    <div class={styles.grid}>
      <For each={props.records}>
        {(record) => (
          <div class={styles.card}>
            <div class={styles.cardHeader}>
              <strong>
                <input
                  type="text"
                  value={getRecordTitle(record, props.columns)}
                  placeholder={t('table.untitled') || 'Untitled'}
                  onBlur={(e) =>
                    props.columns[0] &&
                    updateRecordField(record, props.columns[0].name, e.target.value, props.onUpdateRecord)
                  }
                  onKeyDown={handleEnterBlur}
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
                      value={getFieldValue(record, col.name)}
                      onBlur={(e) => updateRecordField(record, col.name, e.target.value, props.onUpdateRecord)}
                      onKeyDown={handleEnterBlur}
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
