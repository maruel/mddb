import { For, Show, createMemo } from 'solid-js';
import styles from './DatabaseBoard.module.css';

interface Column {
  id: string;
  name: string;
  type: string;
  options?: string[];
}

interface Record {
  id: string;
  data: Record<string, unknown>;
}

interface DatabaseBoardProps {
  records: Record[];
  columns: Column[];
  onDeleteRecord: (id: string) => void;
}

export default function DatabaseBoard(props: DatabaseBoardProps) {
  // Find the first select column to group by
  const groupColumn = () => props.columns.find(c => c.type === 'select' || c.type === 'multi_select');

  const groups = createMemo(() => {
    const col = groupColumn();
    if (!col) return [{ name: 'All Records', records: props.records }];

    const options = col.options || [];
    const grouped: Record<string, { name: string, records: Record[] }> = {};

    // Initialize groups for each option
    options.forEach(opt => {
      grouped[opt] = { name: opt, records: [] };
    });
    // Add "No Group" for records without a value
    grouped['__none__'] = { name: 'No ' + col.name, records: [] };

    props.records.forEach(record => {
      const val = record.data[col.name];
      if (val && typeof val === 'string' && options.includes(val)) {
        grouped[val].records.push(record);
      } else {
        grouped['__none__'].records.push(record);
      }
    });

    return Object.values(grouped).filter(g => g.records.length > 0 || options.includes(g.name));
  });

  return (
    <div class={styles.board}>
      <Show 
        when={groupColumn()} 
        fallback={<div class={styles.noGroup}>Add a "select" column to group by status.</div>}
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
                          <strong>{String(record.data[props.columns[0]?.name] || 'Untitled')}</strong>
                          <button 
                            class={styles.deleteBtn}
                            onClick={() => props.onDeleteRecord(record.id)}
                          >
                            ×
                          </button>
                        </div>
                        <div class={styles.cardBody}>
                          <For each={props.columns.slice(1, 4).filter(c => c.id !== groupColumn()?.id)}>
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
              </div>
            )}
          </For>
        </div>
      </Show>
    </div>
  );
}
