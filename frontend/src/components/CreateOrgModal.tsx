// Modal component for creating a new organization.

import { createSignal, createUniqueId, Show } from 'solid-js';
import { useI18n } from '../i18n';
import { Button, Dialog } from './shared';
import styles from './CreateOrgModal.module.css';

interface CreateOrgData {
  name: string;
}

interface CreateOrgModalProps {
  onClose: () => void;
  onCreate: (data: CreateOrgData) => Promise<void>;
  isFirstOrg?: boolean;
}

export default function CreateOrgModal(props: CreateOrgModalProps) {
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
      ariaLabel={props.isFirstOrg ? t('createOrg.firstOrgTitle') : t('createOrg.title')}
      dismissOnBackdrop={!props.isFirstOrg}
      dismissOnEscape={!props.isFirstOrg}
      onClose={props.onClose}
    >
      <header class={styles.header}>
        <h2>{props.isFirstOrg ? t('createOrg.firstOrgTitle') : t('createOrg.title')}</h2>
        <p>{props.isFirstOrg ? t('createOrg.firstOrgDescription') : t('createOrg.description')}</p>
      </header>

      <Show when={error()}>{(message) => <div class={styles.error}>{message()}</div>}</Show>

      <form onSubmit={handleSubmit}>
        <div class={styles.formGroup}>
          <label for={nameInputId}>{t('createOrg.nameLabel')}</label>
          <input
            id={nameInputId}
            type="text"
            value={name()}
            onInput={(e) => setName(e.target.value)}
            placeholder={t('createOrg.namePlaceholder') || ''}
            autofocus
          />
        </div>

        <div class={styles.actions}>
          <Show when={!props.isFirstOrg}>
            <Button variant="secondary" class={styles.secondaryButton} onClick={props.onClose}>
              {t('common.cancel')}
            </Button>
          </Show>
          <Button
            type="submit"
            variant="primary"
            class={`${styles.primaryButton} ${props.isFirstOrg ? styles.fullWidth : ''}`}
            disabled={!name().trim() || loading()}
          >
            {loading() ? t('common.creating') : t('createOrg.create')}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
