// Creates a workspace and retries activation without repeating acknowledged creation.

import { createSignal, createUniqueId, onCleanup, Show } from "solid-js";
import { useI18n } from "../i18n";
import { Button, Dialog } from "./shared";
import styles from "./CreateWorkspaceModal.module.css";

interface CreateWorkspaceData {
  name: string;
}

interface CreateWorkspaceModalProps {
  onClose: () => void;
  onCreate: (data: CreateWorkspaceData) => Promise<string | null>;
  onOpen: (id: string) => Promise<boolean>;
  isFirstWorkspace?: boolean;
}

export default function CreateWorkspaceModal(props: CreateWorkspaceModalProps) {
  const { t } = useI18n();
  let disposed = false;
  onCleanup(() => {
    disposed = true;
  });
  const [name, setName] = createSignal("");
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [createdId, setCreatedId] = createSignal<string | null>(null);
  const nameInputId = createUniqueId();

  const handleSubmit = async (e: Event) => {
    e.preventDefault();
    if (!name().trim()) return;

    try {
      setLoading(true);
      setError(null);
      let id = createdId();
      if (id === null) {
        id = await props.onCreate({ name: name().trim() });
        if (disposed || id === null) return;
        setCreatedId(id);
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
      ariaLabel={props.isFirstWorkspace ? t("createWorkspace.firstWorkspaceTitle") : t("createWorkspace.title")}
      dismissOnBackdrop={!props.isFirstWorkspace}
      dismissOnEscape={!props.isFirstWorkspace}
      onClose={props.onClose}
    >
      <header class={styles.header}>
        <h2>{props.isFirstWorkspace ? t("createWorkspace.firstWorkspaceTitle") : t("createWorkspace.title")}</h2>
        <p>
          {props.isFirstWorkspace ? t("createWorkspace.firstWorkspaceDescription") : t("createWorkspace.description")}
        </p>
      </header>

      <Show when={error()}>{(message) => <div class={styles.error}>{message()}</div>}</Show>
      <Show when={createdId()}>
        <p role="status">{t("createWorkspace.created")}</p>
      </Show>

      <form onSubmit={handleSubmit}>
        <div class={styles.formGroup}>
          <label for={nameInputId}>{t("createWorkspace.nameLabel")}</label>
          <input
            id={nameInputId}
            type="text"
            value={name()}
            onInput={(e) => setName(e.target.value)}
            placeholder={t("createWorkspace.namePlaceholder") || ""}
            autofocus
            disabled={createdId() !== null}
          />
        </div>

        <div class={styles.actions}>
          <Show when={!props.isFirstWorkspace}>
            <Button variant="secondary" class={styles.secondaryButton} onClick={props.onClose}>
              {t("common.cancel")}
            </Button>
          </Show>
          <Button
            type="submit"
            variant="primary"
            class={`${styles.primaryButton} ${props.isFirstWorkspace ? styles.fullWidth : ""}`}
            disabled={!name().trim() || loading()}
          >
            {loading() ? t("common.loading") : createdId() ? t("recovery.retry") : t("createWorkspace.create")}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
