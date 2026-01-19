import { For, Show, createMemo } from 'solid-js';
import type { DataRecord, Column } from '../types';
import styles from './DatabaseBoard.module.css';
import { useI18n } from '../i18n';

interface DatabaseBoardProps {
  records: DataRecord[];
  columns: Column[];
  onDeleteRecord: (id: string) => void;
}

export default function DatabaseBoard(props: DatabaseBoardProps) {
  const { t } = useI18n();
  // Find the first select column to group by
  const groupColumn = () =>
    props.columns.find((c) => c.type === 'select' || c.type === 'multi_select');

  const groups = createMemo(() => {
    const col = groupColumn();
    if (!col) return [{ name: 'All Records', records: props.records }];

    const options = col.options || [];
    const grouped: Record<string, { name: string; records: DataRecord[] }> = {};

    // Initialize groups for each option
    options.forEach((opt) => {
      grouped[opt] = { name: opt, records: [] };
    });
    // Add "No Group" for records without a value
    grouped['__none__'] = { name: t('database.noGroup') || 'No Group', records: [] };

    props.records.forEach((record) => {
      const val = record.data[col.name];
      if (val && typeof val === 'string' && options.includes(val)) {
        const target = grouped[val];
        if (target) {
          target.records.push(record);
        }
      } else {
        const target = grouped['__none__'];
        if (target) {
          target.records.push(record);
        }
      }
    });

    return Object.values(grouped).filter((g) => g.records.length > 0 || options.includes(g.name));
  });

  return (
    <div class={styles.board}>
      <Show
        when={groupColumn()}
        fallback={<div class={styles.noGroup}>{t('database.addSelectColumn')}</div>}
      >
        <div class={styles.columns}>
          <For each={groups()}>
            {(group) => (
              <div class={styles.column}>
                <div class={styles.columnHeader}>
                  <span class={styles.columnName}>{group.name}</span>
                  <span class={styles.columnCount}>{group.records.length}</span>
                </div>
                <div class={styles.cards}>
                  <For each={group.records}>
                    {(record) => (
                      <div class={styles.card}>
                        <div class={styles.cardHeader}>
                          <strong>
                            {String(
                              (props.columns[0] ? record.data[props.columns[0].name] : null) ||
                                t('database.untitled')
                            )}
                          </strong>
                          <button
                            class={styles.deleteBtn}
                            onClick={() => props.onDeleteRecord(record.id)}
                          >
                            ×
                          </button>
                        </div>
                        <div class={styles.cardBody}>
                          <For
                            each={props.columns
                              .slice(1, 4)
                              .filter((c) => c.id !== groupColumn()?.id)}
                          >
                            {(col) => (
                              <div class={styles.field}>
                                <span class={styles.fieldName}>{col.name}:</span>
                                <span class={styles.fieldValue}>
                                  {String(record.data[col.name] || '-')}
                                </span>
                              </div>
                            )}
                          </For>
                        </div>
                      </div>
                    )}
                  </For>
                </div>
              </div>
            )}
          </For>
        </div>
      </Show>
    </div>
  );
}
