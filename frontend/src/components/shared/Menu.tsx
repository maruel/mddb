// Accessible dropdown menu foundation with arrow navigation and trigger focus restoration.

import { onCleanup, onMount, splitProps, type JSX } from 'solid-js';
import styles from './Menu.module.css';

export interface MenuProps {
  ariaLabel: string;
  children: JSX.Element;
  class?: string;
  onClose: () => void;
  trigger: () => HTMLElement | undefined;
}

export type MenuItemProps = JSX.ButtonHTMLAttributes<HTMLButtonElement>;

function menuItems(menu: HTMLElement): HTMLButtonElement[] {
  return Array.from(menu.querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not([disabled])'));
}

export function Menu(props: MenuProps) {
  let menuRef: HTMLDivElement | undefined;
  const getMenuItems = () => (menuRef ? menuItems(menuRef) : []);

  const focusItem = (offset: number) => {
    const items = getMenuItems();
    if (items.length === 0) return;

    const currentIndex = items.indexOf(document.activeElement as HTMLButtonElement);
    const nextIndex = currentIndex === -1 ? 0 : (currentIndex + offset + items.length) % items.length;
    items[nextIndex]?.focus();
  };

  const handleKeyDown = (event: KeyboardEvent) => {
    switch (event.key) {
      case 'Escape':
        event.preventDefault();
        props.onClose();
        break;
      case 'ArrowDown':
        event.preventDefault();
        focusItem(1);
        break;
      case 'ArrowUp':
        event.preventDefault();
        focusItem(-1);
        break;
      case 'Home':
        event.preventDefault();
        getMenuItems()[0]?.focus();
        break;
      case 'End': {
        event.preventDefault();
        const items = getMenuItems();
        items[items.length - 1]?.focus();
        break;
      }
    }
  };

  onMount(() => {
    getMenuItems()[0]?.focus();
  });

  onCleanup(() => {
    props.trigger()?.focus();
  });

  return (
    <div
      ref={(el) => (menuRef = el)}
      class={`${styles.menu} ${props.class ?? ''}`}
      role="menu"
      aria-label={props.ariaLabel}
      onKeyDown={handleKeyDown}
    >
      {props.children}
    </div>
  );
}

export function MenuItem(props: MenuItemProps) {
  const [local, buttonProps] = splitProps(props, ['type']);
  const type = () => local.type ?? 'button';
  return <button {...buttonProps} type={type()} role="menuitem" />;
}
