// Expandable navigation item for settings sidebar.

import { createSignal, For, Show, createEffect } from 'solid-js';
import type { UnifiedSettingsMatch } from '../../utils/urls';
import type { NavItem } from './SettingsSidebar';
import styles from './SettingsNavItem.module.css';

interface SettingsNavItemProps {
  item: NavItem;
  depth: number;
  isActive: (url: string, route: UnifiedSettingsMatch) => boolean;
  onNavigate: (url: string) => void;
  currentRoute: UnifiedSettingsMatch;
}

export default function SettingsNavItem(props: SettingsNavItemProps) {
  const hasChildren = () => props.item.children && props.item.children.length > 0;

  // Auto-expand if any child is active
  const shouldAutoExpand = (): boolean => {
    if (!hasChildren()) return false;
    const route = props.currentRoute;
    const checkActive = (items: NavItem[]): boolean => {
      for (const item of items) {
        if (props.isActive(item.url, route)) return true;
        if (item.children && checkActive(item.children)) return true;
      }
      return false;
    };
    return checkActive(props.item.children || []);
  };

  const [isExpanded, setIsExpanded] = createSignal(props.depth === 0 || shouldAutoExpand());

  // Update expansion when route changes
  createEffect(() => {
    // Access currentRoute to track changes (triggers reactivity)
    const route = props.currentRoute;
    if (route && shouldAutoExpand()) {
      setIsExpanded(true);
    }
  });

  const handleClick = (e: MouseEvent) => {
    e.preventDefault();
    if (hasChildren() && !props.item.url) {
      // Section header - just toggle expansion
      setIsExpanded(!isExpanded());
    } else if (props.item.url) {
      // Navigate and expand children if any
      if (hasChildren()) {
        setIsExpanded(true);
      }
      props.onNavigate(props.item.url);
    }
  };

  const toggleExpand = (e: MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsExpanded(!isExpanded());
  };

  const paddingLeft = () => `${props.depth * 12 + 16}px`;

  const navItemClass = () => {
    let classes = styles.navItem;
    if (props.isActive(props.item.url, props.currentRoute)) classes += ' ' + styles.active;
    if (props.depth === 0 && hasChildren()) classes += ' ' + styles.section;
    return classes;
  };

  const expandIconClass = () => {
    let classes = styles.expandIcon;
    if (isExpanded()) classes += ' ' + styles.expanded;
    return classes;
  };

  return (
    <div class={styles.navItemWrapper}>
      <a
        href={props.item.url || '#'}
        class={navItemClass()}
        style={{ 'padding-left': paddingLeft() }}
        onClick={handleClick}
      >
        <Show when={hasChildren()} fallback={<span class={styles.expandSpacer} />}>
          <span class={expandIconClass()} onClick={toggleExpand}>
            ▶
          </span>
        </Show>
        <span class={styles.label}>{props.item.label}</span>
      </a>

      <Show when={isExpanded() && hasChildren()}>
        <div class={styles.children}>
          <For each={props.item.children}>
            {(child) => (
              <SettingsNavItem
                item={child}
                depth={props.depth + 1}
                isActive={props.isActive}
                onNavigate={props.onNavigate}
                currentRoute={props.currentRoute}
              />
            )}
          </For>
        </div>
      </Show>
    </div>
  );
}
