// Accessible modal and drawer dialog foundation with focus management.

import { createContext, onCleanup, onMount, useContext, type Accessor, type JSX } from 'solid-js';
import styles from './Dialog.module.css';

type DismissRule = boolean | (() => boolean);

const DialogOverlayTargetContext = createContext<Accessor<HTMLElement | undefined>>(() => undefined);

export interface DialogProps {
  ariaLabel: string;
  children: JSX.Element;
  class?: string;
  dismissOnBackdrop: DismissRule;
  dismissOnEscape: DismissRule;
  onClose: () => void;
  overlayClass?: string;
  variant?: 'modal' | 'drawer';
}

const focusableSelector =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function allowsDismissal(rule: DismissRule): boolean {
  return typeof rule === 'function' ? rule() : rule;
}

function focusableElements(dialog: HTMLElement): HTMLElement[] {
  return Array.from(dialog.querySelectorAll<HTMLElement>(focusableSelector)).filter((el) => !el.hasAttribute('hidden'));
}

/** Returns the current dialog element for overlays that must remain inside its modal tree. */
export function useDialogOverlayTarget(): Accessor<HTMLElement | undefined> {
  return useContext(DialogOverlayTargetContext);
}

export function Dialog(props: DialogProps) {
  let dialogRef: HTMLDivElement | undefined;
  const previousActiveElement = document.activeElement instanceof HTMLElement ? document.activeElement : null;

  const handleBackdropClick = (event: MouseEvent) => {
    if (event.target === event.currentTarget && allowsDismissal(props.dismissOnBackdrop)) {
      props.onClose();
    }
  };

  const handleKeyDown = (event: KeyboardEvent) => {
    if (event.key === 'Escape') {
      event.preventDefault();
      if (allowsDismissal(props.dismissOnEscape)) {
        props.onClose();
      }
      return;
    }

    if (event.key !== 'Tab' || !dialogRef) return;

    const focusable = focusableElements(dialogRef);
    if (focusable.length === 0) {
      event.preventDefault();
      dialogRef.focus();
      return;
    }

    const currentIndex = focusable.indexOf(document.activeElement as HTMLElement);
    if (currentIndex === -1 || document.activeElement === dialogRef) {
      event.preventDefault();
      (event.shiftKey ? focusable[focusable.length - 1] : focusable[0])?.focus();
    } else if (event.shiftKey && currentIndex === 0) {
      event.preventDefault();
      focusable[focusable.length - 1]?.focus();
    } else if (!event.shiftKey && currentIndex === focusable.length - 1) {
      event.preventDefault();
      focusable[0]?.focus();
    }
  };

  onMount(() => {
    const autofocus = dialogRef?.querySelector<HTMLElement>('[autofocus]');
    const firstFocusable = dialogRef ? focusableElements(dialogRef)[0] : undefined;
    (autofocus ?? firstFocusable ?? dialogRef)?.focus();
    document.addEventListener('keydown', handleKeyDown);
  });

  onCleanup(() => {
    document.removeEventListener('keydown', handleKeyDown);
    previousActiveElement?.focus();
  });

  const variant = () => props.variant ?? 'modal';

  return (
    <div
      class={`${styles.overlay} ${variant() === 'drawer' ? styles.drawerOverlay : ''} ${props.overlayClass ?? ''}`}
      onClick={handleBackdropClick}
    >
      <div
        ref={(el) => (dialogRef = el)}
        class={`${styles.dialog} ${variant() === 'drawer' ? styles.drawer : ''} ${props.class ?? ''}`}
        role="dialog"
        aria-label={props.ariaLabel}
        aria-modal="true"
        tabIndex={-1}
      >
        <DialogOverlayTargetContext.Provider value={() => dialogRef}>
          {props.children}
        </DialogOverlayTargetContext.Provider>
      </div>
    </div>
  );
}
