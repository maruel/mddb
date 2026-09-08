// Record detail panel (slide-over) showing all fields of a record for editing.

import { For, Show, createMemo } from 'solid-js';
import type { DataRecordResponse, Property } from '@sdk/types.gen';
import { updateRecordField, handleEnterBlur, getRecordTitle } from './table/tableUtils';
import { FieldEditor } from './table/FieldEditor';
import { useI18n } from '../i18n';
import { Dialog, IconButton } from './shared';
import styles from './RecordDetail.module.css';
import DeleteIcon from '@material-symbols/svg-400/outlined/delete.svg?solid';
import ContentCopyIcon from '@material-symbols/svg-400/outlined/content_copy.svg?solid';

interface RecordDetailProps {
  recordId: string;
  records: DataRecordResponse[];
  columns: Property[];
  onUpdate: (id: string, data: Record<string, unknown>) => void;
  onClose: () => void;
  onDelete?: (id: string) => void;
  onDuplicate?: (id: string) => void;
}

export default function RecordDetail(props: RecordDetailProps) {
  const { t } = useI18n();

  const record = createMemo(() => props.records.find((r) => r.id === props.recordId));

  const titleColumn = () => props.columns[0];
  const bodyColumns = () => props.columns.slice(1);

  return (
    <Dialog
      ariaLabel={t('table.recordDetail')}
      class={styles.panel}
      dismissOnBackdrop={true}
      dismissOnEscape={true}
      onClose={props.onClose}
      variant="drawer"
    >
      <div class={styles.header}>
        <h2 class={styles.headerTitle}>{t('table.recordDetail')}</h2>
        <div class={styles.headerActions}>
          <Show when={props.onDuplicate}>
            <IconButton
              variant="ghost"
              class={styles.actionButton}
              onClick={() => {
                props.onDuplicate?.(props.recordId);
                props.onClose();
              }}
              aria-label={t('table.duplicateRecord')}
              title={t('table.duplicateRecord')}
            >
              <ContentCopyIcon />
            </IconButton>
          </Show>
          <Show when={props.onDelete}>
            <IconButton
              variant="ghost"
              class={styles.actionButton}
              onClick={() => {
                props.onDelete?.(props.recordId);
                props.onClose();
              }}
              aria-label={t('table.deleteRecord')}
              title={t('table.deleteRecord')}
            >
              <DeleteIcon />
            </IconButton>
          </Show>
          <IconButton variant="ghost" class={styles.closeButton} onClick={props.onClose} aria-label={t('common.close')}>
            ×
          </IconButton>
        </div>
      </div>
      <div class={styles.body}>
        <Show when={record()}>
          {(rec) => (
            <>
              <Show when={titleColumn()}>
                {(col) => (
                  <div class={styles.field}>
                    <label class={styles.fieldLabel}>{col().name}</label>
                    <input
                      type="text"
                      value={getRecordTitle(rec(), props.columns)}
                      placeholder={t('table.untitled') || 'Untitled'}
                      onBlur={(e) => updateRecordField(rec(), col().name, e.target.value, props.onUpdate)}
                      onKeyDown={handleEnterBlur}
                      class={styles.titleInput}
                      autofocus
                    />
                  </div>
                )}
              </Show>
              <For each={bodyColumns()}>
                {(col) => (
                  <div class={styles.field}>
                    <label class={styles.fieldLabel}>{col.name}</label>
                    <div class={styles.fieldValue}>
                      <FieldEditor record={rec()} column={col} onUpdate={props.onUpdate} />
                    </div>
                  </div>
                )}
              </For>
            </>
          )}
        </Show>
      </div>
    </Dialog>
  );
}
