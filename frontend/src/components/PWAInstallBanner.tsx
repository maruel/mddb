import { createSignal, onMount, onCleanup, Show } from 'solid-js';
import { useI18n } from '../i18n';
import styles from './PWAInstallBanner.module.css';

interface BeforeInstallPromptEvent extends Event {
  prompt(): Promise<void>;
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>;
}

const STORAGE_KEY = 'mddb_pwa_dismissed';
const DISMISS_DURATION_DAYS = 30;

function isDismissed(): boolean {
  const dismissed = localStorage.getItem(STORAGE_KEY);
  if (!dismissed) return false;

  const dismissedDate = new Date(dismissed);
  const now = new Date();
  const daysDiff = (now.getTime() - dismissedDate.getTime()) / (1000 * 60 * 60 * 24);

  return daysDiff < DISMISS_DURATION_DAYS;
}

export default function PWAInstallBanner() {
  const { t } = useI18n();
  const [deferredPrompt, setDeferredPrompt] = createSignal<BeforeInstallPromptEvent | null>(null);
  const [showBanner, setShowBanner] = createSignal(false);

  onMount(() => {
    if (isDismissed()) return;

    const handleBeforeInstall = (e: Event) => {
      e.preventDefault();
      setDeferredPrompt(e as BeforeInstallPromptEvent);
      setShowBanner(true);
    };

    const handleAppInstalled = () => {
      setShowBanner(false);
      setDeferredPrompt(null);
    };

    window.addEventListener('beforeinstallprompt', handleBeforeInstall);
    window.addEventListener('appinstalled', handleAppInstalled);

    onCleanup(() => {
      window.removeEventListener('beforeinstallprompt', handleBeforeInstall);
      window.removeEventListener('appinstalled', handleAppInstalled);
    });
  });

  const handleInstall = async () => {
    const prompt = deferredPrompt();
    if (!prompt) return;

    await prompt.prompt();
    const { outcome } = await prompt.userChoice;

    if (outcome === 'accepted') {
      setShowBanner(false);
    }
    setDeferredPrompt(null);
  };

  const handleDismiss = () => {
    localStorage.setItem(STORAGE_KEY, new Date().toISOString());
    setShowBanner(false);
  };

  return (
    <Show when={showBanner() && deferredPrompt()}>
      <div class={styles.banner}>
        <div class={styles.content}>
          <div class={styles.title}>{t('pwa.installTitle')}</div>
          <div class={styles.message}>{t('pwa.installMessage')}</div>
        </div>
        <div class={styles.actions}>
          <button class={styles.dismissButton} onClick={handleDismiss}>
            {t('pwa.dismissButton')}
          </button>
          <button class={styles.installButton} onClick={handleInstall}>
            {t('pwa.installButton')}
          </button>
        </div>
      </div>
    </Show>
  );
}
