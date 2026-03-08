// Record detail panel (slide-over) showing all fields of a record for editing.

import { For, Show, createMemo, onMount, onCleanup } from 'solid-js';
import type { DataRecordResponse, Property } from '@sdk/types.gen';
import { updateRecordField, handleEnterBlur, getRecordTitle } from './table/tableUtils';
import { FieldEditor } from './table/FieldEditor';
import { useI18n } from '../i18n';
import styles from './RecordDetail.module.css';

interface RecordDetailProps {
  recordId: string;
  records: DataRecordResponse[];
  columns: Property[];
  onUpdate: (id: string, data: Record<string, unknown>) => void;
  onClose: () => void;
}

export default function RecordDetail(props: RecordDetailProps) {
  const { t } = useI18n();

  const record = createMemo(() => props.records.find((r) => r.id === props.recordId));

  const titleColumn = () => props.columns[0];
  const bodyColumns = () => props.columns.slice(1);

  onMount(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        props.onClose();
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    onCleanup(() => document.removeEventListener('keydown', handleKeyDown));
  });

  return (
    <div class={styles.overlay} onClick={() => props.onClose()}>
      <div class={styles.panel} onClick={(e) => e.stopPropagation()} role="dialog" aria-label={t('table.recordDetail')}>
        <div class={styles.header}>
          <h2 class={styles.headerTitle}>{t('table.recordDetail')}</h2>
          <button class={styles.closeButton} onClick={() => props.onClose()} aria-label={t('common.close')}>
            ×
          </button>
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
      </div>
    </div>
  );
}
