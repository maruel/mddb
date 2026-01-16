import { For, Show } from 'solid-js';
import styles from './DatabaseGallery.module.css';

interface Column {
  id: string;
  name: string;
  type: string;
}

interface Record {
  id: string;
  data: Record<string, unknown>;
}

interface DatabaseGalleryProps {
  records: Record[];
  columns: Column[];
  onDeleteRecord: (id: string) => void;
}

export default function DatabaseGallery(props: DatabaseGalleryProps) {
  // Try to find an image column
  const imageColumn = () => props.columns.find(c => 
    c.name.toLowerCase().includes('image') || 
    c.name.toLowerCase().includes('cover') ||
    c.name.toLowerCase().includes('url')
  );

  return (
    <div class={styles.gallery}>
      <For each={props.records}>
        {(record) => (
          <div class={styles.card}>
            <Show when={imageColumn()}>
              <div class={styles.imageContainer}>
                <Show 
                  when={record.data[imageColumn()!.name]} 
                  fallback={<div class={styles.imagePlaceholder}>No Image</div>}
                >
                  <img 
                    src={String(record.data[imageColumn()!.name])} 
                    alt={String(record.data[props.columns[0]?.name] || 'Record')} 
                    class={styles.image}
                  />
                </Show>
              </div>
            </Show>
            <div class={styles.cardContent}>
              <div class={styles.cardHeader}>
                <strong>{String(record.data[props.columns[0]?.name] || 'Untitled')}</strong>
                <button 
                  class={styles.deleteBtn}
                  onClick={() => props.onDeleteRecord(record.id)}
                >
                  ✕
                </button>
              </div>
              <div class={styles.cardBody}>
                <For each={props.columns.slice(1, 3)}>
                  {(col) => (
                    <div class={styles.field}>
                      <span class={styles.fieldName}>{col.name}:</span>
                      <span class={styles.fieldValue}>{String(record.data[col.name] || '-')}</span>
                    </div>
                  )}
                </For>
              </div>
            </div>
          </div>
        )}
      </For>
    </div>
  );
}
