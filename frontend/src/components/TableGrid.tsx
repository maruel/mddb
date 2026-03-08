// Grid view for table records, displaying data in cards.

import { For, Show } from 'solid-js';
import type { DataRecordResponse, Property } from '@sdk/types.gen';
import { updateRecordField, handleEnterBlur, getRecordTitle, getFieldValue } from './table/tableUtils';
import { TableRow } from './table/TableRow';
import { useI18n } from '../i18n';
import styles from './TableGrid.module.css';

interface TableGridProps {
  records: DataRecordResponse[];
  columns: Property[];
  onAddRecord?: () => void;
  onUpdateRecord?: (id: string, data: Record<string, unknown>) => void;
  onDeleteRecord: (id: string) => void;
  onDuplicateRecord?: (id: string) => void;
  onOpenRecord?: (id: string) => void;
}

export default function TableGrid(props: TableGridProps) {
  const { t } = useI18n();

  return (
    <div class={styles.container}>
      <div class={styles.grid}>
        <For each={props.records}>
          {(record) => (
            <TableRow
              recordId={record.id}
              onDelete={props.onDeleteRecord}
              onDuplicate={props.onDuplicateRecord}
              onOpen={props.onOpenRecord}
              class={styles.card}
            >
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
            </TableRow>
          )}
        </For>
      </div>
      <div class={styles.statusBar}>
        <span>
          {props.records.length} {t('table.recordCount') || 'records'}
        </span>
        <Show when={props.onAddRecord}>
          <button class={styles.addRecord} onClick={() => props.onAddRecord?.()}>
            + {t('table.addRecord') || 'Add Record'}
          </button>
        </Show>
      </div>
    </div>
  );
}
