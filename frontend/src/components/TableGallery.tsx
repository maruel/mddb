// Gallery view for table records, emphasizing images.

import { For, Show } from 'solid-js';
import type { DataRecordResponse, Property } from '@sdk/types.gen';
import { updateRecordField, handleEnterBlur, getRecordTitle, getFieldValue } from './table/tableUtils';
import { TableRow } from './table/TableRow';
import { useI18n } from '../i18n';
import styles from './TableGallery.module.css';

interface TableGalleryProps {
  records: DataRecordResponse[];
  columns: Property[];
  onUpdateRecord?: (id: string, data: Record<string, unknown>) => void;
  onDeleteRecord: (id: string) => void;
  onDuplicateRecord?: (id: string) => void;
  onOpenRecord?: (id: string) => void;
}

export default function TableGallery(props: TableGalleryProps) {
  const { t } = useI18n();

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
            <TableRow
              recordId={record.id}
              onDelete={props.onDeleteRecord}
              onDuplicate={props.onDuplicateRecord}
              onOpen={props.onOpenRecord}
              class={styles.card}
            >
              <Show when={imgCol}>
                {(col) => (
                  <div class={styles.imageContainer}>
                    <Show
                      when={record.data[col().name]}
                      fallback={<div class={styles.imagePlaceholder}>{t('table.noImage')}</div>}
                    >
                      <img
                        src={String(record.data[col().name])}
                        alt={getRecordTitle(record, props.columns) || 'Record'}
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
                  <For each={props.columns.slice(1, 3)}>
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
            </TableRow>
          );
        }}
      </For>
    </div>
  );
}
