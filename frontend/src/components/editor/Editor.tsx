// WYSIWYG markdown editor component using ProseMirror with prosemirror-markdown.

import { createSignal, onMount, onCleanup, createEffect, on, createMemo, Show, untrack } from 'solid-js';
import { EditorView } from 'prosemirror-view';
import type { Node as ProseMirrorNode } from 'prosemirror-model';
import { useI18n } from '../../i18n';
import {
  rewriteAssetUrls,
  reverseRewriteAssetUrls,
  rewriteInternalLinkTitles,
  type NodeTitleMap,
} from './markdown-utils';
import { schema, nodes, marks, markdownParser, markdownSerializer, createEditorState } from './prosemirror-config';
import { createSlashCommandPlugin, type SlashMenuState } from './slashCommandPlugin';
import { createDropUploadPlugin } from './dropUploadPlugin';
import { createInvalidLinkPlugin, updateInvalidLinkState, INTERNAL_LINK_URL_PATTERN } from './invalidLinkPlugin';
import { useAssetUpload, isImageMimeType } from './useAssetUpload';
import SlashCommandMenu from './SlashCommandMenu';
import EditorToolbar, { type FormatState } from './EditorToolbar';
import type { AssetUrlMap } from '../../contexts/EditorContext';
import styles from './Editor.module.css';

interface EditorProps {
  content: string;
  nodeId?: string;
  assetUrls?: AssetUrlMap;
  linkedNodeTitles?: NodeTitleMap;
  onChange: (markdown: string) => void;
  placeholder?: string;
  readOnly?: boolean;
  wsId?: string;
  getToken?: () => string | null;
  onAssetUploaded?: () => void;
  onError?: (error: string) => void;
  onNavigateToNode?: (nodeId: string) => void;
}

export default function Editor(props: EditorProps) {
  const { t } = useI18n();
  const [editorMode, setEditorMode] = createSignal<'wysiwyg' | 'markdown'>('wysiwyg');
  // Use untrack for initial value - we don't want to re-create the signal when props.content changes
  const [markdownContent, setMarkdownContent] = createSignal(untrack(() => props.content));
  const [view, setView] = createSignal<EditorView | undefined>();
  const [slashMenuState, setSlashMenuState] = createSignal<SlashMenuState>({
    active: false,
    query: '',
    triggerPos: 0,
    position: { top: 0, left: 0 },
  });
  let editorRef: HTMLDivElement | undefined;

  // Create slash command plugin with callback
  const slashPlugin = createSlashCommandPlugin(setSlashMenuState);

  // Create invalid link decoration plugin
  const invalidLinkPlugin = createInvalidLinkPlugin();

  // Handle file drop uploads
  const handleFileDrop = async (files: File[], pos: number) => {
    const editorView = view();
    if (!editorView || !props.wsId || !props.nodeId || !props.getToken) return;

    const { uploadFile, error } = useAssetUpload({
      wsId: props.wsId,
      nodeId: props.nodeId,
      getToken: props.getToken,
    });

    for (const file of files) {
      const result = await uploadFile(file);
      if (result) {
        const isImage = isImageMimeType(result.mimeType);
        let tr = editorView.state.tr;

        if (isImage) {
          // Insert image node with signed URL for immediate display.
          // reverseRewriteAssetUrls will extract the filename when saving.
          const imageType = schema.nodes.image;
          if (imageType) {
            const imageNode = imageType.create({
              src: result.url,
              alt: result.name,
              title: null,
            });
            tr = tr.insert(pos, imageNode);
          }
        } else {
          // Insert link with signed URL for immediate display.
          // reverseRewriteAssetUrls will extract the filename when saving.
          const linkType = schema.marks.link;
          if (linkType) {
            const linkMark = linkType.create({ href: result.url, title: null });
            const textNode = schema.text(result.name, [linkMark]);
            tr = tr.insert(pos, textNode);
          }
        }

        editorView.dispatch(tr);

        // Notify parent to reload node (updates asset URLs)
        props.onAssetUploaded?.();
      } else {
        // Report error to parent
        const errMsg = error();
        if (errMsg) {
          props.onError?.(errMsg);
        }
      }
    }
  };

  // Create drop upload plugin
  const dropPlugin = createDropUploadPlugin({ onFileDrop: handleFileDrop });

  // Track what we've emitted to distinguish our own changes from external updates
  // Use untrack for initial values - these are bookkeeping variables, not reactive bindings
  let lastLoadedNodeId: string | undefined = untrack(() => props.nodeId);
  let lastEmittedContent: string = untrack(() => props.content);
  let lastLinkedNodeTitles: NodeTitleMap | undefined = untrack(() => props.linkedNodeTitles);

  // Track active formatting states for toolbar buttons
  const [formatState, setFormatState] = createSignal<FormatState>({
    isBold: false,
    isItalic: false,
    isUnderline: false,
    isCode: false,
    headingLevel: null,
    isBulletList: false,
    isOrderedList: false,
    isTaskList: false,
    isBlockquote: false,
    isCodeBlock: false,
  });

  const [toolbarPosition, setToolbarPosition] = createSignal<{ top: number; left: number } | null>(null);

  // Parse markdown to ProseMirror document, handling asset URLs and link titles
  const parseMarkdown = (md: string): ProseMirrorNode | null => {
    let processed = rewriteAssetUrls(md, props.assetUrls || {});
    // Rewrite internal link titles to current titles if wsId is available
    if (props.wsId && props.linkedNodeTitles && Object.keys(props.linkedNodeTitles).length > 0) {
      processed = rewriteInternalLinkTitles(processed, props.linkedNodeTitles, props.wsId);
    }
    return markdownParser.parse(processed);
  };

  // Serialize ProseMirror document to markdown, handling asset URLs
  const serializeMarkdown = (doc: ProseMirrorNode): string => {
    const md = markdownSerializer.serialize(doc);
    return reverseRewriteAssetUrls(md, props.assetUrls || {});
  };

  // Update active state signals based on current selection
  const updateActiveStates = (editorView: EditorView) => {
    const { state } = editorView;
    const { from, $from, to, empty } = state.selection;

    // Update toolbar position
    if (empty || editorMode() === 'markdown') {
      setToolbarPosition(null);
    } else {
      try {
        const start = editorView.coordsAtPos(from);
        const end = editorView.coordsAtPos(to);

        // Calculate horizontal center
        // If on same line (approx), center between start and end
        const isSameLine = Math.abs(start.top - end.top) < 20;
        let left = start.left;
        if (isSameLine) {
          left = (start.left + end.right) / 2;
        } else {
          // Multi-line: position near the start
          left = start.left + 40;
        }

        setToolbarPosition({
          top: start.top,
          left,
        });
      } catch {
        setToolbarPosition(null);
      }
    }

    // Check marks
    const currentMarks = empty ? state.storedMarks || $from.marks() : [];
    const isBold = marks.strong.isInSet(currentMarks) !== undefined || state.doc.rangeHasMark(from, to, marks.strong);
    const isItalic = marks.em.isInSet(currentMarks) !== undefined || state.doc.rangeHasMark(from, to, marks.em);
    const isUnderline =
      marks.underline.isInSet(currentMarks) !== undefined || state.doc.rangeHasMark(from, to, marks.underline);
    const isCode = marks.code.isInSet(currentMarks) !== undefined || state.doc.rangeHasMark(from, to, marks.code);

    // Check block types
    const node = $from.node($from.depth);
    const headingLevel = node.type === nodes.heading ? (node.attrs.level as number) : null;
    const isCodeBlock = node.type === nodes.code_block;

    // Check list types by walking up ancestors
    let isBulletList = false;
    let isOrderedList = false;
    let isTaskList = false;
    let isBlockquote = false;
    for (let d = $from.depth; d > 0; d--) {
      const n = $from.node(d);
      if (n.type === nodes.bullet_list) isBulletList = true;
      if (n.type === nodes.ordered_list) isOrderedList = true;
      if (n.type === nodes.list_item && n.attrs.checked !== null) isTaskList = true;
      if (n.type === nodes.blockquote) isBlockquote = true;
    }
    // Also check nodes in selection range (for Ctrl+A selection where $from is at doc root)
    if (!isBulletList && !isOrderedList && !isTaskList) {
      state.doc.nodesBetween(from, to, (n) => {
        if (n.type === nodes.bullet_list) isBulletList = true;
        if (n.type === nodes.ordered_list) isOrderedList = true;
        if (n.type === nodes.list_item && n.attrs.checked !== null) isTaskList = true;
        if (n.type === nodes.blockquote) isBlockquote = true;
      });
    }
    // Task lists are implemented as bullet lists with checked attrs, so they're mutually exclusive
    if (isTaskList) {
      isBulletList = false;
    }

    setFormatState({
      isBold,
      isItalic,
      isUnderline,
      isCode,
      headingLevel,
      isBulletList,
      isOrderedList,
      isTaskList,
      isBlockquote,
      isCodeBlock,
    });
  };

  onMount(() => {
    if (!editorRef) return;

    const doc = parseMarkdown(props.content);
    if (!doc) return;
    const state = createEditorState(doc, [slashPlugin, dropPlugin, invalidLinkPlugin]);

    const editorView = new EditorView(editorRef, {
      state,
      editable: () => !props.readOnly,
      dispatchTransaction(tr) {
        const newState = editorView.state.apply(tr);
        editorView.updateState(newState);

        if (tr.docChanged) {
          const md = serializeMarkdown(newState.doc);
          setMarkdownContent(md);
          lastEmittedContent = md;
          props.onChange(md);
        }

        updateActiveStates(editorView);
      },
      handleDOMEvents: {
        click: (_view, event) => {
          // Handle clicks on links
          const target = event.target as HTMLElement;
          const anchor = target.closest('a');
          if (!anchor) return false;

          const href = anchor.getAttribute('href');
          if (!href) return false;

          // Check if it's an internal page link
          const match = href.match(INTERNAL_LINK_URL_PATTERN);
          if (match && match[2] && props.onNavigateToNode) {
            event.preventDefault();
            props.onNavigateToNode(match[2]);
            return true;
          }

          // External links (http/https) open in new window
          if (href.startsWith('http://') || href.startsWith('https://')) {
            event.preventDefault();
            window.open(href, '_blank', 'noopener,noreferrer');
            return true;
          }

          return false;
        },
      },
    });

    // Store view reference on DOM element for checkbox plugin access
    const pmEl = editorRef.querySelector('.ProseMirror') as HTMLElement & { pmView?: EditorView };
    if (pmEl) {
      pmEl.pmView = editorView;
    }

    setView(editorView);
    updateActiveStates(editorView);

    // Initialize invalid link decorations with current titles (even if empty, to detect invalid links)
    updateInvalidLinkState(editorView, props.linkedNodeTitles || {}, props.wsId);
  });

  onCleanup(() => {
    view()?.destroy();
  });

  // Sync when node changes, content changes externally, or linked node titles are fetched
  createEffect(
    on(
      () => [props.nodeId, props.content, props.linkedNodeTitles] as const,
      ([nodeId, content, linkedNodeTitles]) => {
        const nodeChanged = nodeId !== lastLoadedNodeId;
        const contentChangedExternally = content !== lastEmittedContent;
        const titlesChanged = linkedNodeTitles !== lastLinkedNodeTitles;

        if (nodeChanged || contentChangedExternally || titlesChanged) {
          lastLoadedNodeId = nodeId;
          lastEmittedContent = content;
          lastLinkedNodeTitles = linkedNodeTitles;

          // Only reset markdown content and mode when node or content changes, not just titles
          if (nodeChanged || contentChangedExternally) {
            setMarkdownContent(content);
            setEditorMode('wysiwyg');
          }

          const editorView = view();
          if (editorView) {
            const doc = parseMarkdown(content);
            if (doc) {
              const state = createEditorState(doc, [slashPlugin, dropPlugin, invalidLinkPlugin]);
              editorView.updateState(state);
              // Update invalid link decorations with current titles
              updateInvalidLinkState(editorView, linkedNodeTitles || {}, props.wsId);
            }
          }
        }
      },
      { defer: true }
    )
  );

  const handleMarkdownChange = (value: string) => {
    setMarkdownContent(value);
    lastEmittedContent = value;
    props.onChange(value);
  };

  const switchToWysiwyg = () => {
    const editorView = view();
    if (editorView) {
      const doc = parseMarkdown(markdownContent());
      if (doc) {
        const state = createEditorState(doc, [slashPlugin, dropPlugin, invalidLinkPlugin]);
        editorView.updateState(state);
        updateActiveStates(editorView);
        // Update invalid link decorations with current titles
        updateInvalidLinkState(editorView, props.linkedNodeTitles || {}, props.wsId);
      }
    }
    setEditorMode('wysiwyg');
  };

  const switchToMarkdown = () => {
    const editorView = view();
    if (editorView) {
      const md = serializeMarkdown(editorView.state.doc);
      setMarkdownContent(md);
    }
    setEditorMode('markdown');
  };

  const toggleMode = () => {
    if (editorMode() === 'wysiwyg') {
      switchToMarkdown();
    } else {
      switchToWysiwyg();
    }
  };

  // Keyboard shortcut: Ctrl+Shift+M to toggle mode
  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.ctrlKey && e.shiftKey && e.key.toLowerCase() === 'm') {
      e.preventDefault();
      toggleMode();
    }
  };

  onMount(() => {
    document.addEventListener('keydown', handleKeyDown);
  });

  onCleanup(() => {
    document.removeEventListener('keydown', handleKeyDown);
  });

  const wysiwygClass = createMemo(() =>
    editorMode() === 'wysiwyg' ? styles.prosemirrorEditor : `${styles.prosemirrorEditor} ${styles.hidden}`
  );

  const markdownClass = createMemo(() =>
    editorMode() === 'markdown' ? styles.markdownEditor : `${styles.markdownEditor} ${styles.hidden}`
  );

  return (
    <div class={styles.editorContainer}>
      <EditorToolbar formatState={formatState()} view={view()} position={toolbarPosition()} />

      <div ref={editorRef} class={wysiwygClass()} data-testid="wysiwyg-editor" />

      <textarea
        class={markdownClass()}
        value={markdownContent()}
        onInput={(e) => handleMarkdownChange(e.target.value)}
        placeholder={props.placeholder || t('editor.contentPlaceholder') || 'Write markdown...'}
        readOnly={props.readOnly}
        data-testid="markdown-editor"
      />

      {/* Mode toggle at bottom-right */}
      <div class={styles.modeIndicator}>
        <button
          class={editorMode() === 'wysiwyg' ? styles.modeIndicatorActive : undefined}
          onClick={switchToWysiwyg}
          title="Visual mode (Ctrl+Shift+M)"
          data-testid="editor-mode-visual"
        >
          Visual
        </button>
        <button
          class={editorMode() === 'markdown' ? styles.modeIndicatorActive : undefined}
          onClick={switchToMarkdown}
          title="Markdown mode (Ctrl+Shift+M)"
          data-testid="editor-mode-markdown"
        >
          MD
        </button>
      </div>

      <Show when={view()}>{(v) => <SlashCommandMenu view={v()} state={slashMenuState()} nodeId={props.nodeId} />}</Show>
    </div>
  );
}
