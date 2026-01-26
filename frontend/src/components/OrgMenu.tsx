// Dropdown menu for switching between organizations.

import { createSignal, For, onMount, onCleanup, Show } from 'solid-js';
import { useI18n } from '../i18n';
import type { OrgMembershipResponse } from '../types.gen';
import styles from './OrgMenu.module.css';

interface OrgMenuProps {
  memberships: OrgMembershipResponse[];
  currentOrgId: string;
  onSwitchOrg: (orgId: string) => void;
  onCreateOrg: () => void;
}

export default function OrgMenu(props: OrgMenuProps) {
  const { t } = useI18n();
  const [isOpen, setIsOpen] = createSignal(false);
  let menuRef: HTMLDivElement | undefined;

  const currentOrgName = () => {
    const current = props.memberships.find((m) => m.organization_id === props.currentOrgId);
    return current?.organization_name || t('app.myOrg');
  };

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

  const handleSwitchOrg = (orgId: string) => {
    setIsOpen(false);
    if (orgId !== props.currentOrgId) {
      props.onSwitchOrg(orgId);
    }
  };

  const handleCreateOrg = () => {
    setIsOpen(false);
    props.onCreateOrg();
  };

  return (
    <div class={styles.orgMenu} ref={menuRef}>
      <button class={styles.orgButton} onClick={() => setIsOpen(!isOpen())} title={currentOrgName()}>
        <span class={styles.orgName}>{currentOrgName()}</span>
        <span class={styles.chevron}>{isOpen() ? '▲' : '▼'}</span>
      </button>

      <Show when={isOpen()}>
        <div class={styles.dropdown}>
          <div class={styles.orgList}>
            <For each={props.memberships}>
              {(m) => (
                <button
                  class={`${styles.orgItem} ${m.organization_id === props.currentOrgId ? styles.active : ''}`}
                  onClick={() => handleSwitchOrg(m.organization_id)}
                >
                  <span class={styles.orgItemName}>{m.organization_name || m.organization_id}</span>
                  <Show when={m.organization_id === props.currentOrgId}>
                    <span class={styles.checkmark}>✓</span>
                  </Show>
                </button>
              )}
            </For>
          </div>
          <div class={styles.divider} />
          <button class={styles.createOrgItem} onClick={handleCreateOrg}>
            <span class={styles.plusIcon}>+</span>
            {t('createOrg.title')}
          </button>
        </div>
      </Show>
    </div>
  );
}
