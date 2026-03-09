// Kanban board view for table records, grouped by select/multi-select columns.

import { For, Show, createMemo } from 'solid-js';
import { type DataRecordResponse, type Property, PropertyTypeSelect, PropertyTypeMultiSelect } from '@sdk/types.gen';
import { updateRecordField, handleEnterBlur, getRecordTitle } from './table/tableUtils';
import { FieldEditor } from './table/FieldEditor';
import { TableRow } from './table/TableRow';
import { useI18n } from '../i18n';
import styles from './TableBoard.module.css';

interface TableBoardProps {
  records: DataRecordResponse[];
  columns: Property[];
  onAddRecord?: (data: Record<string, unknown>) => void;
  onUpdateRecord?: (id: string, data: Record<string, unknown>) => void;
  onDeleteRecord: (id: string) => void;
  onDuplicateRecord?: (id: string) => void;
  onOpenRecord?: (id: string) => void;
  groupByColumn?: string;
  onGroupByChange?: (columnName: string) => void;
  onUpdateColumn?: (col: Property) => void;
}

export default function TableBoard(props: TableBoardProps) {
  const { t } = useI18n();

  // Columns eligible for grouping.
  const groupableColumns = createMemo(() =>
    props.columns.filter((c) => c.type === PropertyTypeSelect || c.type === PropertyTypeMultiSelect)
  );

  const groupColumn = createMemo(() => {
    const cols = groupableColumns();
    if (cols.length === 0) return undefined;
    if (props.groupByColumn) {
      const found = cols.find((c) => c.name === props.groupByColumn);
      if (found) return found;
    }
    return cols[0];
  });

  const groups = createMemo(() => {
    const col = groupColumn();
    if (!col) return [{ id: '', name: 'All Records', color: undefined, records: props.records }];

    const grouped: Record<string, { id: string; name: string; color?: string; records: DataRecordResponse[] }> = {};

    if (col.options) {
      col.options.forEach((opt) => {
        grouped[opt.id] = { id: opt.id, name: opt.name, color: opt.color, records: [] };
      });
    }

    grouped['__none__'] = { id: '__none__', name: t('table.noGroup') || 'No Group', color: undefined, records: [] };

    props.records.forEach((record) => {
      const val = record.data[col.name];
      if (val && typeof val === 'string') {
        if (!grouped[val]) {
          grouped[val] = { id: val, name: val, color: undefined, records: [] };
        }
        grouped[val].records.push(record);
      } else {
        grouped['__none__']?.records.push(record);
      }
    });

    return Object.values(grouped).filter((g) => g.records.length > 0 || g.id !== '__none__');
  });

  // Body columns: all except the title column and the group column.
  const bodyColumns = createMemo(() => {
    const col = groupColumn();
    const titleCol = props.columns[0];
    return props.columns.filter((c) => c !== titleCol && c !== col);
  });

  const handleAddCard = (groupId: string) => {
    const col = groupColumn();
    if (!props.onAddRecord) return;
    const data: Record<string, unknown> = {};
    if (col && groupId !== '__none__') {
      data[col.name] = groupId;
    }
    props.onAddRecord(data);
  };

  return (
    <div class={styles.board} data-testid="board">
      <Show when={groupColumn()} fallback={<div class={styles.noGroup}>{t('table.addSelectColumn')}</div>}>
        <Show when={groupableColumns().length > 1 && props.onGroupByChange}>
          <div class={styles.boardHeader}>
            <label class={styles.groupByLabel} for="board-group-by">
              {t('table.groupBy')}:
            </label>
            <select
              id="board-group-by"
              class={styles.groupBySelect}
              value={groupColumn()?.name ?? ''}
              onChange={(e) => props.onGroupByChange?.(e.currentTarget.value)}
            >
              <For each={groupableColumns()}>{(col) => <option value={col.name}>{col.name}</option>}</For>
            </select>
          </div>
        </Show>
        <div class={styles.columns}>
          <For each={groups()}>
            {(group) => (
              <div
                class={styles.column}
                data-testid="board-column"
                style={group.color ? { '--column-color': group.color } : {}}
              >
                <div class={styles.columnHeader} data-testid="board-column-header">
                  <div class={styles.columnTitle}>
                    <span class={styles.columnName}>{group.name}</span>
                  </div>
                  <span class={styles.columnCount}>{group.records.length}</span>
                </div>
                <div class={styles.cards}>
                  <For each={group.records}>
                    {(record) => (
                      <TableRow
                        recordId={record.id}
                        onDelete={props.onDeleteRecord}
                        onDuplicate={props.onDuplicateRecord}
                        onOpen={props.onOpenRecord}
                        class={styles.card}
                      >
                        <div class={styles.cardHeader}>
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
                        </div>
                        <Show when={bodyColumns().length > 0}>
                          <div class={styles.cardBody}>
                            <For each={bodyColumns()}>
                              {(col) => (
                                <div class={styles.field}>
                                  <span class={styles.fieldName}>{col.name}</span>
                                  <span class={styles.fieldValue}>
                                    <FieldEditor
                                      record={record}
                                      column={col}
                                      onUpdate={props.onUpdateRecord}
                                      onUpdateOptions={(opts) => props.onUpdateColumn?.({ ...col, options: opts })}
                                    />
                                  </span>
                                </div>
                              )}
                            </For>
                          </div>
                        </Show>
                      </TableRow>
                    )}
                  </For>
                  <Show when={props.onAddRecord}>
                    <button class={styles.addCard} onClick={() => handleAddCard(group.id)}>
                      + {t('table.addRecord') || 'Add'}
                    </button>
                  </Show>
                </div>
              </div>
            )}
          </For>
        </div>
      </Show>
    </div>
  );
}
