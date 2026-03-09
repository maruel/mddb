// Icon picker component for selecting emoji (Noto Color Emoji) or Material Symbols icons.

import { createSignal, For, Show, onMount, onCleanup, createResource } from 'solid-js';
import { useI18n } from '../../i18n';
import styles from './IconPicker.module.css';

// Emoji entry from the generated groups file.
interface EmojiEntry {
  c: string; // emoji character
  n: string; // Unicode name (used for search)
}

interface EmojiGroup {
  g: string; // group name
  e: EmojiEntry[];
}

// Lazy-load emoji groups (Unicode-classified, ~132 KB JSON) on first Emoji tab open.
async function loadEmojiGroups(): Promise<EmojiGroup[]> {
  const mod = await import('./emoji-groups.json');
  return mod.default as EmojiGroup[];
}

// Lazy-load Material Symbols icon names (3,798 names, ~58 KB JSON) on first Icons tab open.
async function loadIconNames(): Promise<string[]> {
  const mod = await import('./material-symbols-names.json');
  return mod.default as string[];
}

interface IconPickerProps {
  onSelect: (icon: string) => void;
  onRemove: () => void;
  onClose: () => void;
  hasIcon: boolean;
}

export function IconPicker(props: IconPickerProps) {
  const { t } = useI18n();
  const [tab, setTab] = createSignal<'emoji' | 'icons'>('emoji');
  const [search, setSearch] = createSignal('');
  let containerRef: HTMLDivElement | undefined;

  const [emojiGroups] = createResource(
    () => tab() === 'emoji',
    (active) => (active ? loadEmojiGroups() : Promise.resolve([] as EmojiGroup[]))
  );

  const [iconNames] = createResource(
    () => tab() === 'icons',
    (active) => (active ? loadIconNames() : Promise.resolve([] as string[]))
  );

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Escape') props.onClose();
  };

  onMount(() => {
    document.addEventListener('keydown', handleKeyDown);
    const handleClickOutside = (e: MouseEvent) => {
      if (containerRef && !containerRef.contains(e.target as Node)) {
        props.onClose();
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    onCleanup(() => {
      document.removeEventListener('keydown', handleKeyDown);
      document.removeEventListener('mousedown', handleClickOutside);
    });
  });

  const filteredEmojiGroups = () => {
    const groups = emojiGroups() ?? [];
    const q = search().toLowerCase();
    if (!q) return groups;
    // Search by emoji name or group name.
    return groups
      .map((g) => ({
        ...g,
        e: g.e.filter((em) => em.n.includes(q) || g.g.toLowerCase().includes(q)),
      }))
      .filter((g) => g.e.length > 0);
  };

  const filteredIcons = () => {
    const names = iconNames() ?? [];
    const q = search().toLowerCase().replace(/\s+/g, '_');
    if (!q) return names;
    return names.filter((n) => n.includes(q));
  };

  return (
    <div class={styles.picker} ref={(el) => (containerRef = el)}>
      <div class={styles.pickerHeader}>
        <span class={styles.pickerTitle}>{t('editor.iconPickerTitle')}</span>
        <Show when={props.hasIcon}>
          <button class={styles.removeBtn} onClick={() => props.onRemove()}>
            {t('editor.removeIcon')}
          </button>
        </Show>
      </div>
      <div class={styles.tabs}>
        <button
          class={styles.tab}
          classList={{ [`${styles.activeTab}`]: tab() === 'emoji' }}
          onClick={() => setTab('emoji')}
        >
          {t('editor.iconPickerEmoji')}
        </button>
        <button
          class={styles.tab}
          classList={{ [`${styles.activeTab}`]: tab() === 'icons' }}
          onClick={() => setTab('icons')}
        >
          {t('editor.iconPickerIcons')}
        </button>
      </div>
      <input
        class={styles.searchInput}
        type="text"
        placeholder={t('editor.iconPickerSearch') || 'Search...'}
        value={search()}
        onInput={(e) => setSearch(e.target.value)}
        autofocus
      />
      <div class={styles.grid}>
        <Show when={tab() === 'emoji'}>
          <Show when={!emojiGroups.loading} fallback={<div class={styles.loadingMsg}>Loading…</div>}>
            <For each={filteredEmojiGroups()}>
              {(group) => (
                <>
                  <div class={styles.groupLabel}>{group.g}</div>
                  <div class={styles.emojiRow}>
                    <For each={group.e}>
                      {(em) => (
                        <button class={styles.emojiBtn} onClick={() => props.onSelect(em.c)} title={em.n}>
                          {em.c}
                        </button>
                      )}
                    </For>
                  </div>
                </>
              )}
            </For>
          </Show>
        </Show>
        <Show when={tab() === 'icons'}>
          <Show when={!iconNames.loading} fallback={<div class={styles.loadingMsg}>Loading…</div>}>
            <div class={styles.iconGrid}>
              <For each={filteredIcons()}>
                {(name) => (
                  <button class={styles.iconBtn} onClick={() => props.onSelect(name)} title={name.replace(/_/g, ' ')}>
                    <span class="material-symbols-outlined">{name}</span>
                  </button>
                )}
              </For>
            </div>
          </Show>
        </Show>
      </div>
    </div>
  );
}

/** Renders an icon value (emoji or Material Symbols icon name) as a display element. */
export function IconDisplay(props: { icon: string; class?: string }) {
  const isEmoji = () => {
    const c = props.icon.codePointAt(0) ?? 0;
    return c > 255;
  };

  return (
    <Show when={props.icon}>
      <Show
        when={isEmoji()}
        fallback={<span class={`material-symbols-outlined ${props.class ?? ''}`}>{props.icon}</span>}
      >
        <span class={`${styles.emojiDisplay} ${props.class ?? ''}`} aria-label={props.icon}>
          {props.icon}
        </span>
      </Show>
    </Show>
  );
}
