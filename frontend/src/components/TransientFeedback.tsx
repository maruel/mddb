// Fixed live-region feedback for transient workspace operation failures.

import { onCleanup, onMount } from 'solid-js';
import { useI18n } from '../i18n';
import { Button } from './shared';
import styles from './TransientFeedback.module.css';

const DISMISS_AFTER_MS = 8_000;

interface TransientFeedbackProps {
  message: string;
  onDismiss: () => void;
}

export function TransientFeedback(props: TransientFeedbackProps) {
  const { t } = useI18n();
  let dismissTimer: number | undefined;

  onMount(() => {
    dismissTimer = window.setTimeout(props.onDismiss, DISMISS_AFTER_MS);
  });

  onCleanup(() => {
    if (dismissTimer !== undefined) {
      window.clearTimeout(dismissTimer);
    }
  });

  return (
    <div class={styles.feedback} role="alert" aria-live="assertive" aria-atomic="true" data-testid="workspace-feedback">
      <span class={styles.message}>{props.message}</span>
      <Button variant="ghost" class={styles.dismiss} onClick={props.onDismiss}>
        {t('common.close') || 'Close'}
      </Button>
    </div>
  );
}
