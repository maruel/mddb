// Gallery view for table records, emphasizing images.

import { For, Show } from 'solid-js';
import type { DataRecordResponse, Property } from '../types.gen';
import styles from './TableGallery.module.css';
import { useI18n } from '../i18n';

interface TableGalleryProps {
  records: DataRecordResponse[];
  columns: Property[];
  onUpdateRecord?: (id: string, data: Record<string, unknown>) => void;
  onDeleteRecord: (id: string) => void;
}

export default function TableGallery(props: TableGalleryProps) {
  const { t } = useI18n();

  const handleUpdate = (record: DataRecordResponse, colName: string, value: string) => {
    if (record.data[colName] === value || !props.onUpdateRecord) return;
    const newData = { ...record.data, [colName]: value };
    props.onUpdateRecord(record.id, newData);
  };
  // Try to find an image column
  const imageColumn = () =>
    props.columns.find(
      (c) =>
        c.name.toLowerCase().includes('image') ||
        c.name.toLowerCase().includes('cover') ||
        c.name.toLowerCase().includes('url')
    );

  return (
    <div class={styles.gallery}>
      <For each={props.records}>
        {(record) => {
          const imgCol = imageColumn();
          return (
            <div class={styles.card}>
              <Show when={imgCol}>
                {(col) => (
                  <div class={styles.imageContainer}>
                    <Show
                      when={record.data[col().name]}
                      fallback={<div class={styles.imagePlaceholder}>{t('table.noImage')}</div>}
                    >
                      <img
                        src={String(record.data[col().name])}
                        alt={String((props.columns[0] ? record.data[props.columns[0].name] : null) || 'Record')}
                        class={styles.image}
                      />
                    </Show>
                  </div>
                )}
              </Show>
              <div class={styles.cardContent}>
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
                  <For each={props.columns.slice(1, 3)}>
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
            </div>
          );
        }}
      </For>
    </div>
  );
}
