// Modal for importing a workspace from Notion.

import { createSignal, onCleanup, onMount } from 'solid-js';
import { useI18n } from '../i18n';
import styles from './NotionImportModal.module.css';

export interface NotionImportData {
  notionToken: string;
}

interface NotionImportModalProps {
  onClose: () => void;
  onImport: (data: NotionImportData) => Promise<void>;
}

const NOTION_INTEGRATION_URL = 'https://www.notion.so/profile/integrations/form/new-integration';

export default function NotionImportModal(props: NotionImportModalProps) {
  const { t } = useI18n();
  const [notionToken, setNotionToken] = createSignal('');
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Escape' && !loading()) {
      props.onClose();
    }
  };

  onMount(() => {
    document.addEventListener('keydown', handleKeyDown);
  });

  onCleanup(() => {
    document.removeEventListener('keydown', handleKeyDown);
  });

  const handleSubmit = async (e: Event) => {
    e.preventDefault();
    if (!notionToken().trim()) return;

    try {
      setLoading(true);
      setError(null);
      await props.onImport({
        notionToken: notionToken().trim(),
      });
      props.onClose();
    } catch (err) {
      setError(String(err));
    } finally {
      setLoading(false);
    }
  };

  const handleOverlayClick = (e: MouseEvent) => {
    if (e.target === e.currentTarget) {
      props.onClose();
    }
  };

  return (
    <div class={styles.overlay} onClick={handleOverlayClick}>
      <div class={styles.modal}>
        <header class={styles.header}>
          <h2>{t('notionImport.title')}</h2>
          <p>{t('notionImport.description')}</p>
        </header>

        {error() && <div class={styles.error}>{error()}</div>}

        <div class={styles.setupBox}>
          <div class={styles.setupHeader}>
            <span class={styles.setupTitle}>{t('notionImport.setupTitle')}</span>
            <a href={NOTION_INTEGRATION_URL} target="_blank" rel="noopener noreferrer" class={styles.createLink}>
              {t('notionImport.createIntegration')} &#x2197;
            </a>
          </div>
          <ol class={styles.setupSteps}>
            <li>
              {t('notionImport.step1Header')}
              <ol class={styles.subSteps}>
                <li>{t('notionImport.step1a')}</li>
                <li>{t('notionImport.step1b')}</li>
                <li>{t('notionImport.step1c')}</li>
                <li>{t('notionImport.step1d')}</li>
              </ol>
            </li>
            <li>
              {t('notionImport.step2Header')}
              <ol class={styles.subSteps}>
                <li>{t('notionImport.step2a')}</li>
                <li>{t('notionImport.step2b')}</li>
              </ol>
            </li>
            <li>
              {t('notionImport.step3Header')}
              <ol class={styles.subSteps}>
                <li>{t('notionImport.step3a')}</li>
                <li>{t('notionImport.step3b')}</li>
              </ol>
            </li>
          </ol>
        </div>

        <form onSubmit={handleSubmit}>
          <div class={styles.formGroup}>
            <label>{t('notionImport.notionToken')}</label>
            <input
              type="password"
              value={notionToken()}
              onInput={(e) => setNotionToken(e.target.value)}
              placeholder={t('notionImport.notionTokenPlaceholder') || ''}
              autofocus
            />
          </div>

          <div class={styles.actions}>
            <button type="button" class={styles.secondaryButton} onClick={props.onClose}>
              {t('common.cancel')}
            </button>
            <button type="submit" class={styles.primaryButton} disabled={!notionToken().trim() || loading()}>
              {loading() ? t('notionImport.importing') : t('notionImport.startImport')}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
