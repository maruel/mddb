// List view for table records, displaying each record as a compact horizontal row.

import { For, Show } from 'solid-js';
import type { DataRecordResponse, Property } from '@sdk/types.gen';
import { getRecordTitle } from './table/tableUtils';
import { FieldValue } from './table/FieldValue';
import { TableRow } from './table/TableRow';
import { useI18n } from '../i18n';
import styles from './TableList.module.css';

interface TableListProps {
  records: DataRecordResponse[];
  columns: Property[];
  onAddRecord?: () => void;
  onUpdateRecord?: (id: string, data: Record<string, unknown>) => void;
  onDeleteRecord: (id: string) => void;
  onDuplicateRecord?: (id: string) => void;
  onOpenRecord?: (id: string) => void;
}

export default function TableList(props: TableListProps) {
  const { t } = useI18n();

  const bodyColumns = () => props.columns.slice(1);

  return (
    <div class={styles.container} data-testid="list-view">
      <Show when={props.records.length > 0} fallback={<div class={styles.empty}>{t('table.noRecords')}</div>}>
        <div class={styles.list}>
          <For each={props.records}>
            {(record) => (
              <TableRow
                recordId={record.id}
                onDelete={props.onDeleteRecord}
                onDuplicate={props.onDuplicateRecord}
                onOpen={props.onOpenRecord}
                class={styles.row}
              >
                <span class={styles.title}>
                  {getRecordTitle(record, props.columns) || t('table.untitled') || 'Untitled'}
                </span>
                <Show when={bodyColumns().length > 0}>
                  <span class={styles.fields}>
                    <For each={bodyColumns()}>
                      {(col) => {
                        const val = record.data[col.name];
                        if (val === undefined || val === null || val === '') return null;
                        return (
                          <span class={styles.field}>
                            <span class={styles.fieldName}>{col.name}</span>
                            <span class={styles.fieldValue}>
                              <FieldValue record={record} column={col} />
                            </span>
                          </span>
                        );
                      }}
                    </For>
                  </span>
                </Show>
              </TableRow>
            )}
          </For>
        </div>
      </Show>
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
