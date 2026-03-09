// Gallery view for table records, emphasizing images.

import { For, Show } from 'solid-js';
import type { DataRecordResponse, Property } from '@sdk/types.gen';
import { PropertyTypeURL } from '@sdk/types.gen';
import { updateRecordField, handleEnterBlur, getRecordTitle } from './table/tableUtils';
import { FieldEditor } from './table/FieldEditor';
import { TableRow } from './table/TableRow';
import { useI18n } from '../i18n';
import styles from './TableGallery.module.css';

interface TableGalleryProps {
  records: DataRecordResponse[];
  columns: Property[];
  onAddRecord?: () => void;
  onUpdateRecord?: (id: string, data: Record<string, unknown>) => void;
  onDeleteRecord: (id: string) => void;
  onDuplicateRecord?: (id: string) => void;
  onOpenRecord?: (id: string) => void;
  onUpdateColumn?: (col: Property) => void;
}

export default function TableGallery(props: TableGalleryProps) {
  const { t } = useI18n();

  // Find the first URL-type column to use as a cover image; fall back to name heuristic.
  const imageColumn = () =>
    props.columns.find((c) => c.type === PropertyTypeURL) ??
    props.columns.find(
      (c) =>
        c.name.toLowerCase().includes('image') ||
        c.name.toLowerCase().includes('cover') ||
        c.name.toLowerCase().includes('photo')
    );

  const titleColumn = () => props.columns[0];

  // All columns except the title and the image column (shown separately).
  const bodyColumns = () => {
    const imgCol = imageColumn();
    const titleCol = titleColumn();
    return props.columns.filter((c) => c !== titleCol && c !== imgCol);
  };

  return (
    <div class={styles.container} data-testid="gallery-view">
      <Show when={props.records.length > 0} fallback={<div class={styles.empty}>{t('table.noRecords')}</div>}>
        <div class={styles.gallery} data-testid="gallery">
          <For each={props.records}>
            {(record) => {
              const imgCol = imageColumn();
              const titleCol = titleColumn();
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
                            onError={(e) => {
                              e.currentTarget.style.display = 'none';
                              const sibling = e.currentTarget.nextSibling as HTMLElement | null;
                              sibling?.style.setProperty('display', 'flex');
                            }}
                          />
                          <div class={styles.imagePlaceholder} style={{ display: 'none' }}>
                            {t('table.noImage')}
                          </div>
                        </Show>
                      </div>
                    )}
                  </Show>
                  <div class={styles.cardContent}>
                    <Show when={titleCol}>
                      {(col) => (
                        <div class={styles.cardHeader}>
                          <input
                            type="text"
                            value={getRecordTitle(record, props.columns)}
                            placeholder={t('table.untitled') || 'Untitled'}
                            onBlur={(e) => updateRecordField(record, col().name, e.target.value, props.onUpdateRecord)}
                            onKeyDown={handleEnterBlur}
                            class={styles.titleInput}
                          />
                        </div>
                      )}
                    </Show>
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
                  </div>
                </TableRow>
              );
            }}
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
