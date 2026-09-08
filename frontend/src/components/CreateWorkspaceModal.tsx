// Modal component for creating a new workspace.

import { createSignal, createUniqueId, Show } from 'solid-js';
import { useI18n } from '../i18n';
import { Button, Dialog } from './shared';
import styles from './CreateWorkspaceModal.module.css';

interface CreateWorkspaceData {
  name: string;
}

interface CreateWorkspaceModalProps {
  onClose: () => void;
  onCreate: (data: CreateWorkspaceData) => Promise<void>;
  isFirstWorkspace?: boolean;
}

export default function CreateWorkspaceModal(props: CreateWorkspaceModalProps) {
  const { t } = useI18n();
  const [name, setName] = createSignal('');
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const nameInputId = createUniqueId();

  const handleSubmit = async (e: Event) => {
    e.preventDefault();
    if (!name().trim()) return;

    try {
      setLoading(true);
      setError(null);
      await props.onCreate({
        name: name().trim(),
      });
      props.onClose();
    } catch (err) {
      setError(String(err));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Dialog
      ariaLabel={props.isFirstWorkspace ? t('createWorkspace.firstWorkspaceTitle') : t('createWorkspace.title')}
      dismissOnBackdrop={!props.isFirstWorkspace}
      dismissOnEscape={!props.isFirstWorkspace}
      onClose={props.onClose}
    >
      <header class={styles.header}>
        <h2>{props.isFirstWorkspace ? t('createWorkspace.firstWorkspaceTitle') : t('createWorkspace.title')}</h2>
        <p>
          {props.isFirstWorkspace ? t('createWorkspace.firstWorkspaceDescription') : t('createWorkspace.description')}
        </p>
      </header>

      <Show when={error()}>{(message) => <div class={styles.error}>{message()}</div>}</Show>

      <form onSubmit={handleSubmit}>
        <div class={styles.formGroup}>
          <label for={nameInputId}>{t('createWorkspace.nameLabel')}</label>
          <input
            id={nameInputId}
            type="text"
            value={name()}
            onInput={(e) => setName(e.target.value)}
            placeholder={t('createWorkspace.namePlaceholder') || ''}
            autofocus
          />
        </div>

        <div class={styles.actions}>
          <Show when={!props.isFirstWorkspace}>
            <Button variant="secondary" class={styles.secondaryButton} onClick={props.onClose}>
              {t('common.cancel')}
            </Button>
          </Show>
          <Button
            type="submit"
            variant="primary"
            class={`${styles.primaryButton} ${props.isFirstWorkspace ? styles.fullWidth : ''}`}
            disabled={!name().trim() || loading()}
          >
            {loading() ? t('common.creating') : t('createWorkspace.create')}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
