// Modal reference for workspace navigation keyboard shortcuts.

import { For } from 'solid-js';
import { useI18n } from '../i18n';
import { Button, Dialog } from './shared';
import styles from './KeyboardShortcutsDialog.module.css';

interface KeyboardShortcutsDialogProps {
  onClose: () => void;
}

export default function KeyboardShortcutsDialog(props: KeyboardShortcutsDialogProps) {
  const { t } = useI18n();
  const shortcuts = () => [
    { key: '?', description: t('app.showKeyboardShortcuts') },
    { key: 'G', description: t('app.focusWorkspaceTree') },
    { key: '↑ ↓', description: t('app.moveThroughWorkspacePages') },
    { key: 'Enter', description: t('app.openFocusedWorkspacePage') },
  ];

  return (
    <Dialog
      ariaLabel={t('app.keyboardShortcuts')}
      dismissOnBackdrop={true}
      dismissOnEscape={true}
      onClose={props.onClose}
    >
      <header class={styles.header}>
        <h2>{t('app.keyboardShortcuts')}</h2>
        <p>{t('app.keyboardShortcutsDescription')}</p>
      </header>
      <dl class={styles.shortcutList}>
        <For each={shortcuts()}>
          {(shortcut) => (
            <div class={styles.shortcut}>
              <dt>
                <kbd>{shortcut.key}</kbd>
              </dt>
              <dd>{shortcut.description}</dd>
            </div>
          )}
        </For>
      </dl>
      <div class={styles.actions}>
        <Button variant="secondary" onClick={props.onClose}>
          {t('common.close')}
        </Button>
      </div>
    </Dialog>
  );
}
