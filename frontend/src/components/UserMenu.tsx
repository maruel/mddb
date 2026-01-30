// Dropdown menu for user profile and logout.

import { createSignal, Show } from 'solid-js';
import { useI18n } from '../i18n';
import { useAuth } from '../contexts';
import { useClickOutside } from '../composables/useClickOutside';
import styles from './UserMenu.module.css';

interface UserMenuProps {
  onProfile: () => void;
}

export default function UserMenu(props: UserMenuProps) {
  const { t } = useI18n();
  const { user, logout } = useAuth();
  const [isOpen, setIsOpen] = createSignal(false);
  let menuRef: HTMLDivElement | undefined;

  // Get avatar URL from OAuth identities (prefer first one with avatar)
  const getAvatarUrl = () => {
    const u = user();
    if (!u) return null;
    const identities = u.oauth_identities;
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
    const u = user();
    if (!u) return '';
    const name = u.name || u.email || '';
    const parts = name.split(/[\s@]+/);
    if (parts.length >= 2 && parts[0] && parts[1]) {
      return ((parts[0][0] || '') + (parts[1][0] || '')).toUpperCase();
    }
    return name.slice(0, 2).toUpperCase();
  };

  const userName = () => user()?.name || '';
  const userEmail = () => user()?.email || '';
  const workspaceRole = () => user()?.workspace_role;

  useClickOutside(
    () => menuRef,
    () => setIsOpen(false)
  );

  const handleLogout = () => {
    setIsOpen(false);
    logout();
  };

  const handleProfile = () => {
    setIsOpen(false);
    props.onProfile();
  };

  return (
    <div class={styles.userMenu} ref={menuRef}>
      <button
        class={styles.avatarButton}
        onClick={() => setIsOpen(!isOpen())}
        title={userName() || userEmail()}
        aria-label={t('userMenu.profile') || 'User menu'}
        aria-expanded={isOpen()}
        aria-haspopup="menu"
        data-testid="user-menu-button"
      >
        <Show when={getAvatarUrl()} fallback={<span class={styles.initials}>{getInitials()}</span>}>
          {(url) => (
            <img src={url()} alt={userName() || 'User'} class={styles.avatarImage} referrerPolicy="no-referrer" />
          )}
        </Show>
      </button>

      <Show when={isOpen()}>
        <div class={styles.dropdown} role="menu">
          <div class={styles.userInfo}>
            <span class={styles.userName}>{userName()}</span>
            <span class={styles.userEmail}>{userEmail()}</span>
            <Show when={workspaceRole()}>
              <span class={styles.userRole}>{workspaceRole()}</span>
            </Show>
          </div>
          <div class={styles.divider} />
          <button class={styles.menuItem} onClick={handleProfile} role="menuitem">
            {t('userMenu.profile')}
          </button>
          <button class={styles.menuItem} onClick={handleLogout} role="menuitem">
            {t('userMenu.logout')}
          </button>
        </div>
      </Show>
    </div>
  );
}
