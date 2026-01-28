// Composable for detecting clicks outside an element.

import { onMount, onCleanup } from 'solid-js';

/**
 * Calls onClose when a click occurs outside the referenced element.
 *
 * @param getRef - Function returning the element to watch (or undefined if not mounted)
 * @param onClose - Callback to invoke when clicking outside
 *
 * @example
 * let menuRef: HTMLDivElement | undefined;
 * useClickOutside(() => menuRef, () => setIsOpen(false));
 * // ...
 * <div ref={menuRef}>...</div>
 */
export function useClickOutside(getRef: () => HTMLElement | undefined, onClose: () => void): void {
  onMount(() => {
    const handleClickOutside = (e: MouseEvent) => {
      const el = getRef();
      if (el && !el.contains(e.target as Node)) {
        onClose();
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    onCleanup(() => {
      document.removeEventListener('mousedown', handleClickOutside);
    });
  });
}
