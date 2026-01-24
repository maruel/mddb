import { createSignal, onMount, onCleanup, Show } from 'solid-js';
import { useI18n } from '../i18n';
import type { UserResponse } from '../types.gen';
import styles from './UserMenu.module.css';

interface UserMenuProps {
  user: UserResponse;
  onLogout: () => void;
}

export default function UserMenu(props: UserMenuProps) {
  const { t } = useI18n();
  const [isOpen, setIsOpen] = createSignal(false);
  let menuRef: HTMLDivElement | undefined;

  // Get avatar URL from OAuth identities (prefer first one with avatar)
  const getAvatarUrl = () => {
    const identities = props.user.oauth_identities;
    if (!identities) return null;
    for (const identity of identities) {
      if (identity.avatar_url) {
        return identity.avatar_url;
      }
    }
    return null;
  };

  // Get initials from user name
  const getInitials = () => {
    const name = props.user.name || props.user.email || '';
    const parts = name.split(/[\s@]+/);
    if (parts.length >= 2 && parts[0] && parts[1]) {
      return ((parts[0][0] || '') + (parts[1][0] || '')).toUpperCase();
    }
    return name.slice(0, 2).toUpperCase();
  };

  // Click outside to close
  const handleClickOutside = (e: MouseEvent) => {
    if (menuRef && !menuRef.contains(e.target as Node)) {
      setIsOpen(false);
    }
  };

  onMount(() => {
    document.addEventListener('mousedown', handleClickOutside);
    onCleanup(() => {
      document.removeEventListener('mousedown', handleClickOutside);
    });
  });

  const handleLogout = () => {
    setIsOpen(false);
    props.onLogout();
  };

  return (
    <div class={styles.userMenu} ref={menuRef}>
      <button
        class={styles.avatarButton}
        onClick={() => setIsOpen(!isOpen())}
        title={props.user.name || props.user.email}
      >
        <Show when={getAvatarUrl()} fallback={<span class={styles.initials}>{getInitials()}</span>}>
          {(url) => (
            <img src={url()} alt={props.user.name || 'User'} class={styles.avatarImage} referrerPolicy="no-referrer" />
          )}
        </Show>
      </button>

      <Show when={isOpen()}>
        <div class={styles.dropdown}>
          <div class={styles.userInfo}>
            <span class={styles.userName}>{props.user.name}</span>
            <span class={styles.userEmail}>{props.user.email}</span>
            <Show when={props.user.workspace_role}>
              <span class={styles.userRole}>{props.user.workspace_role}</span>
            </Show>
          </div>
          <div class={styles.divider} />
          <button class={styles.menuItem} onClick={handleLogout}>
            {t('userMenu.logout')}
          </button>
        </div>
      </Show>
    </div>
  );
}
