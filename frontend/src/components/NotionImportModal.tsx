// Starts a Notion import and retries workspace activation without repeating the import.

import { createSignal, createUniqueId, Show, onCleanup } from "solid-js";
import { useI18n } from "../i18n";
import { Button, Dialog } from "./shared";
import styles from "./NotionImportModal.module.css";

export interface NotionImportData {
  notionToken: string;
}

interface NotionImportModalProps {
  onClose: () => void;
  onImport: (data: NotionImportData) => Promise<string | null>;
  onOpen: (id: string) => Promise<boolean>;
}

const NOTION_INTEGRATION_URL = "https://www.notion.so/profile/integrations/form/new-integration";

export default function NotionImportModal(props: NotionImportModalProps) {
  const { t } = useI18n();
  let disposed = false;
  onCleanup(() => {
    disposed = true;
  });
  const [notionToken, setNotionToken] = createSignal("");
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [startedId, setStartedId] = createSignal<string | null>(null);
  const notionTokenInputId = createUniqueId();

  const handleSubmit = async (e: Event) => {
    e.preventDefault();
    if (!notionToken().trim()) return;

    try {
      setLoading(true);
      setError(null);
      let id = startedId();
      if (id === null) {
        id = await props.onImport({ notionToken: notionToken().trim() });
        if (disposed || id === null) return;
        setStartedId(id);
      }
      if ((await props.onOpen(id)) && !disposed) props.onClose();
    } catch (err) {
      if (!disposed) setError(String(err));
    } finally {
      if (!disposed) setLoading(false);
    }
  };

  return (
    <Dialog
      ariaLabel={t("notionImport.title")}
      class={styles.modal}
      dismissOnBackdrop={true}
      dismissOnEscape={() => !loading()}
      onClose={props.onClose}
    >
      <header class={styles.header}>
        <h2>{t("notionImport.title")}</h2>
        <p>{t("notionImport.description")}</p>
      </header>

      <Show when={error()}>{(message) => <div class={styles.error}>{message()}</div>}</Show>
      <Show when={startedId()}>
        <p role="status">{t("notionImport.started")}</p>
      </Show>

      <div class={styles.setupBox}>
        <div class={styles.setupHeader}>
          <span class={styles.setupTitle}>{t("notionImport.setupTitle")}</span>
          <a href={NOTION_INTEGRATION_URL} target="_blank" rel="noopener noreferrer" class={styles.createLink}>
            {t("notionImport.createIntegration")} &#x2197;
          </a>
        </div>
        <ol class={styles.setupSteps}>
          <li>
            {t("notionImport.step1Header")}
            <ol class={styles.subSteps}>
              <li>{t("notionImport.step1a")}</li>
              <li>{t("notionImport.step1b")}</li>
              <li>{t("notionImport.step1c")}</li>
              <li>{t("notionImport.step1d")}</li>
            </ol>
          </li>
          <li>
            {t("notionImport.step2Header")}
            <ol class={styles.subSteps}>
              <li>{t("notionImport.step2a")}</li>
              <li>{t("notionImport.step2b")}</li>
            </ol>
          </li>
          <li>
            {t("notionImport.step3Header")}
            <ol class={styles.subSteps}>
              <li>{t("notionImport.step3a")}</li>
              <li>{t("notionImport.step3b")}</li>
            </ol>
          </li>
        </ol>
      </div>

      <form onSubmit={handleSubmit}>
        <div class={styles.formGroup}>
          <label for={notionTokenInputId}>{t("notionImport.notionToken")}</label>
          <input
            id={notionTokenInputId}
            type="password"
            value={notionToken()}
            onInput={(e) => setNotionToken(e.target.value)}
            placeholder={t("notionImport.notionTokenPlaceholder") || ""}
            autofocus
            disabled={startedId() !== null}
          />
        </div>

        <div class={styles.actions}>
          <Button variant="secondary" class={styles.secondaryButton} onClick={props.onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            type="submit"
            variant="primary"
            class={styles.primaryButton}
            disabled={!notionToken().trim() || loading()}
          >
            {loading() ? t("common.loading") : startedId() ? t("recovery.retry") : t("notionImport.startImport")}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
