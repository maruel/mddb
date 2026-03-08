// Grid view for table records, displaying data in cards.

import { For, Show } from 'solid-js';
import type { DataRecordResponse, Property } from '@sdk/types.gen';
import { updateRecordField, handleEnterBlur, getRecordTitle } from './table/tableUtils';
import { FieldEditor } from './table/FieldEditor';
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

  const titleColumn = () => props.columns[0];
  const bodyColumns = () => props.columns.slice(1);

  return (
    <div class={styles.container} data-testid="list">
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
                <Show when={titleColumn()}>
                  {(col) => (
                    <input
                      type="text"
                      value={getRecordTitle(record, props.columns)}
                      placeholder={t('table.untitled') || 'Untitled'}
                      onBlur={(e) => updateRecordField(record, col().name, e.target.value, props.onUpdateRecord)}
                      onKeyDown={handleEnterBlur}
                      class={styles.titleInput}
                    />
                  )}
                </Show>
                <Show when={!titleColumn()}>
                  <input
                    type="text"
                    value=""
                    placeholder={t('table.untitled') || 'Untitled'}
                    class={styles.titleInput}
                    disabled
                  />
                </Show>
              </div>
              <Show when={bodyColumns().length > 0}>
                <div class={styles.cardBody}>
                  <For each={bodyColumns()}>
                    {(col) => (
                      <div class={styles.field}>
                        <span class={styles.fieldName}>{col.name}</span>
                        <FieldEditor record={record} column={col} onUpdate={props.onUpdateRecord} />
                      </div>
                    )}
                  </For>
                </div>
              </Show>
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
