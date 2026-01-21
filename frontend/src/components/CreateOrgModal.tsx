import { createSignal, Show } from 'solid-js';
import { useI18n } from '../i18n';
import styles from './Onboarding.module.css';

interface CreateOrgData {
  name: string;
  welcomePageTitle: string;
  welcomePageContent: string;
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

  const handleSubmit = async (e: Event) => {
    e.preventDefault();
    if (!name().trim()) return;

    try {
      setLoading(true);
      setError(null);
      await props.onCreate({
        name: name().trim(),
        welcomePageTitle: t('createOrg.welcomeTitle') || 'Welcome',
        welcomePageContent: t('createOrg.welcomeContent') || '# Welcome\n\nYour new workspace.',
      });
      props.onClose();
    } catch (err) {
      setError(String(err));
    } finally {
      setLoading(false);
    }
  };

  const handleOverlayClick = (e: MouseEvent) => {
    // Only allow closing by clicking overlay if not first org
    if (!props.isFirstOrg && e.target === e.currentTarget) {
      props.onClose();
    }
  };

  return (
    <div class={styles.overlay} onClick={handleOverlayClick}>
      <div class={styles.modal}>
        <header class={styles.header}>
          <h2>{props.isFirstOrg ? t('createOrg.firstOrgTitle') : t('createOrg.title')}</h2>
          <p>
            {props.isFirstOrg ? t('createOrg.firstOrgDescription') : t('createOrg.description')}
          </p>
        </header>

        {error() && <div class={styles.error}>{error()}</div>}

        <form onSubmit={handleSubmit}>
          <div class={styles.formGroup}>
            <label>{t('createOrg.nameLabel')}</label>
            <input
              type="text"
              value={name()}
              onInput={(e) => setName(e.target.value)}
              placeholder={t('createOrg.namePlaceholder') || ''}
              autofocus
            />
          </div>

          <div class={styles.actions}>
            <Show when={!props.isFirstOrg}>
              <button type="button" class={styles.secondaryButton} onClick={props.onClose}>
                {t('common.cancel')}
              </button>
            </Show>
            <button
              type="submit"
              class={styles.primaryButton}
              disabled={!name().trim() || loading()}
              style={props.isFirstOrg ? { flex: 1 } : undefined}
            >
              {loading() ? t('common.creating') : t('createOrg.create')}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
