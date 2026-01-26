// Modal component for creating a new workspace.

import { createSignal, Show } from 'solid-js';
import { useI18n } from '../i18n';
import styles from './Onboarding.module.css';

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

  const handleOverlayClick = (e: MouseEvent) => {
    // Only allow closing by clicking overlay if not first workspace
    if (!props.isFirstWorkspace && e.target === e.currentTarget) {
      props.onClose();
    }
  };

  return (
    <div class={styles.overlay} onClick={handleOverlayClick}>
      <div class={styles.modal}>
        <header class={styles.header}>
          <h2>{props.isFirstWorkspace ? t('createWorkspace.firstWorkspaceTitle') : t('createWorkspace.title')}</h2>
          <p>
            {props.isFirstWorkspace ? t('createWorkspace.firstWorkspaceDescription') : t('createWorkspace.description')}
          </p>
        </header>

        {error() && <div class={styles.error}>{error()}</div>}

        <form onSubmit={handleSubmit}>
          <div class={styles.formGroup}>
            <label>{t('createWorkspace.nameLabel')}</label>
            <input
              type="text"
              value={name()}
              onInput={(e) => setName(e.target.value)}
              placeholder={t('createWorkspace.namePlaceholder') || ''}
              autofocus
            />
          </div>

          <div class={styles.actions}>
            <Show when={!props.isFirstWorkspace}>
              <button type="button" class={styles.secondaryButton} onClick={props.onClose}>
                {t('common.cancel')}
              </button>
            </Show>
            <button
              type="submit"
              class={styles.primaryButton}
              disabled={!name().trim() || loading()}
              style={props.isFirstWorkspace ? { flex: 1 } : undefined}
            >
              {loading() ? t('common.creating') : t('createWorkspace.create')}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
