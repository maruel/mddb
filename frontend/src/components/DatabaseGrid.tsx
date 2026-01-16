import { For } from 'solid-js';
import styles from './DatabaseGrid.module.css';

interface Column {
  id: string;
  name: string;
  type: string;
}

interface Record {
  id: string;
  data: Record<string, unknown>;
}

interface DatabaseGridProps {
  records: Record[];
  columns: Column[];
  onDeleteRecord: (id: string) => void;
}

export default function DatabaseGrid(props: DatabaseGridProps) {
  return (
    <div class={styles.grid}>
      <For each={props.records}>
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
