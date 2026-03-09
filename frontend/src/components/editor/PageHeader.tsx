// Page header component: cover image and icon display with edit UX.

import { createSignal, Show } from 'solid-js';
import { useI18n } from '../../i18n';
import { IconPicker, IconDisplay } from './IconPicker';
import AddPhotoIcon from '@material-symbols/svg-400/outlined/add_photo_alternate.svg?solid';
import styles from './PageHeader.module.css';

interface PageHeaderProps {
  icon: string;
  cover: string;
  coverUrl: string; // signed URL for cover asset
  onIconChange: (icon: string) => void;
  onCoverChange: (cover: string) => void;
  onUploadCover: (file: File) => Promise<string>; // returns new asset filename
}

export function PageHeader(props: PageHeaderProps) {
  const { t } = useI18n();
  const [showIconPicker, setShowIconPicker] = createSignal(false);
  const [draggingCover, setDraggingCover] = createSignal(false);
  let coverInputRef: HTMLInputElement | undefined;

  const hasCover = () => !!props.cover && !!props.coverUrl;
  const hasIcon = () => !!props.icon;

  async function handleCoverFileInput(file: File) {
    const filename = await props.onUploadCover(file);
    if (filename) {
      props.onCoverChange(filename);
    }
  }

  function handleCoverDrop(e: DragEvent) {
    e.preventDefault();
    setDraggingCover(false);
    const file = e.dataTransfer?.files?.[0];
    if (file && file.type.startsWith('image/')) {
      handleCoverFileInput(file);
    }
  }

  return (
    <div class={styles.pageHeader}>
      {/* Cover image */}
      <Show when={hasCover()}>
        <div
          class={styles.coverWrapper}
          classList={{ [`${styles.coverDragging}`]: draggingCover() }}
          onDragOver={(e) => {
            e.preventDefault();
            setDraggingCover(true);
          }}
          onDragLeave={() => setDraggingCover(false)}
          onDrop={handleCoverDrop}
        >
          <img class={styles.coverImage} src={props.coverUrl} alt="" />
          <div class={styles.coverOverlay}>
            <button class={styles.coverBtn} onClick={() => coverInputRef?.click()}>
              {t('editor.changeCover')}
            </button>
            <button class={styles.coverBtn} onClick={() => props.onCoverChange('')}>
              {t('editor.removeCover')}
            </button>
          </div>
        </div>
      </Show>

      {/* Icon and hover-actions row */}
      <div class={styles.metaRow}>
        {/* Icon */}
        <Show when={hasIcon()}>
          <div class={styles.iconWrapper}>
            <button
              class={styles.iconButton}
              onClick={() => setShowIconPicker((v) => !v)}
              title={t('editor.changeIcon') || 'Change icon'}
            >
              <IconDisplay icon={props.icon} class={styles.pageIcon} />
            </button>
            <Show when={showIconPicker()}>
              <IconPicker
                hasIcon={true}
                onSelect={(v) => {
                  props.onIconChange(v);
                  setShowIconPicker(false);
                }}
                onRemove={() => {
                  props.onIconChange('');
                  setShowIconPicker(false);
                }}
                onClose={() => setShowIconPicker(false)}
              />
            </Show>
          </div>
        </Show>

        {/* Hover action buttons (shown when no icon or no cover) */}
        <div class={styles.hoverActions}>
          <Show when={!hasIcon()}>
            <button class={styles.hoverBtn} onClick={() => setShowIconPicker((v) => !v)}>
              {t('editor.addIcon')}
            </button>
            <Show when={showIconPicker()}>
              <IconPicker
                hasIcon={false}
                onSelect={(v) => {
                  props.onIconChange(v);
                  setShowIconPicker(false);
                }}
                onRemove={() => setShowIconPicker(false)}
                onClose={() => setShowIconPicker(false)}
              />
            </Show>
          </Show>
          <Show when={!hasCover()}>
            <button class={styles.hoverBtn} onClick={() => coverInputRef?.click()}>
              <AddPhotoIcon />
              {t('editor.addCover')}
            </button>
          </Show>
        </div>
      </div>

      {/* Hidden file input for cover upload */}
      <input
        ref={(el) => (coverInputRef = el)}
        type="file"
        accept="image/*"
        style={{ display: 'none' }}
        onChange={(e) => {
          const file = e.target.files?.[0];
          if (file) handleCoverFileInput(file);
          e.target.value = '';
        }}
      />
    </div>
  );
}
