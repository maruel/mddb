// Banner showing Notion import progress and completion status.

import { Show } from 'solid-js';
import { useI18n } from '../i18n';
import type { NotionImportStatusResponse } from '@sdk/types.gen';
import styles from './NotionImportBanner.module.css';

interface NotionImportBannerProps {
  status: NotionImportStatusResponse;
  onCancel: () => void;
  onDismiss: () => void;
}

export default function NotionImportBanner(props: NotionImportBannerProps) {
  const { t } = useI18n();
  const s = () => props.status;

  const bannerClass = () => {
    switch (s().status) {
      case 'completed':
        return `${styles.banner} ${styles.success}`;
      case 'failed':
        return `${styles.banner} ${styles.error}`;
      case 'cancelled':
        return `${styles.banner} ${styles.cancelled}`;
      default:
        return styles.banner;
    }
  };

  const statusMessage = () => {
    switch (s().status) {
      case 'running':
        return t('notionImport.importInProgress');
      case 'completed':
        return t('notionImport.importCompleted');
      case 'failed':
        return t('notionImport.importFailed');
      case 'cancelled':
        return t('notionImport.importCancelled');
      default:
        return '';
    }
  };

  const icon = () => {
    switch (s().status) {
      case 'running':
        return <span class={styles.spinner}>&#x21BB;</span>;
      case 'completed':
        return <span>&#x2713;</span>;
      case 'failed':
        return <span>&#x2717;</span>;
      case 'cancelled':
        return <span>&#x26A0;</span>;
      default:
        return null;
    }
  };

  const isTerminal = () => ['completed', 'failed', 'cancelled'].includes(s().status);

  return (
    <div class={bannerClass()}>
      <span class={styles.icon}>{icon()}</span>

      <div class={styles.content}>
        <span class={styles.message}>{statusMessage()}</span>

        <Show when={s().status === 'running' && s().message}>
          <span class={styles.progress}>
            {s().progress}/{s().total}: {s().message}
          </span>
        </Show>

        <Show when={isTerminal() && (s().pages || s().databases || s().records)}>
          <div class={styles.stats}>
            <Show when={s().pages}>
              <span class={styles.stat}>
                <span class={styles.statValue}>{s().pages}</span> {t('notionImport.pagesImported')}
              </span>
            </Show>
            <Show when={s().databases}>
              <span class={styles.stat}>
                <span class={styles.statValue}>{s().databases}</span> {t('notionImport.databasesImported')}
              </span>
            </Show>
            <Show when={s().records}>
              <span class={styles.stat}>
                <span class={styles.statValue}>{s().records}</span> {t('notionImport.recordsImported')}
              </span>
            </Show>
            <Show when={s().assets}>
              <span class={styles.stat}>
                <span class={styles.statValue}>{s().assets}</span> {t('notionImport.assetsImported')}
              </span>
            </Show>
            <Show when={s().errors}>
              <span class={styles.stat}>
                <span class={styles.statValue}>{s().errors}</span> {t('notionImport.errorsEncountered')}
              </span>
            </Show>
          </div>
        </Show>

        <Show when={s().status === 'failed' && s().message}>
          <span class={styles.progress}>{s().message}</span>
        </Show>
      </div>

      <div class={styles.actions}>
        <Show when={s().status === 'running'}>
          <button class={styles.cancelButton} onClick={() => props.onCancel()}>
            {t('notionImport.cancel')}
          </button>
        </Show>
        <Show when={isTerminal()}>
          <button class={styles.dismissButton} onClick={() => props.onDismiss()}>
            {t('notionImport.dismiss')}
          </button>
        </Show>
      </div>
    </div>
  );
}
