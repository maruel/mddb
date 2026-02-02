// WYSIWYG markdown editor component using ProseMirror with flat block architecture.

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
import { nodes, marks, createEditorState, schema } from './prosemirror-config';
import { parseMarkdown } from './markdown-parser';
import { serializeToMarkdown } from './markdown-serializer';
import { createBlockNodeView } from './BlockNodeView';
import { createSlashCommandPlugin, type SlashMenuState } from './slashCommandPlugin';
import { createDropUploadPlugin } from './dropUploadPlugin';
import { createInvalidLinkPlugin, updateInvalidLinkState, INTERNAL_LINK_URL_PATTERN } from './invalidLinkPlugin';
import { useAssetUpload, isImageMimeType } from './useAssetUpload';
import { BlockContextMenu } from './BlockContextMenu';
import { EditorDropIndicator } from './EditorDropIndicator';
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
  const [markdownContent, setMarkdownContent] = createSignal(untrack(() => props.content));
  const [view, setView] = createSignal<EditorView | undefined>();
  const [slashMenuState, setSlashMenuState] = createSignal<SlashMenuState>({
    active: false,
    query: '',
    triggerPos: 0,
    position: { top: 0, left: 0 },
  });
  const [editorRef, setEditorRef] = createSignal<HTMLDivElement>();

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
          // Flattened block schemas handle images inside blocks or as separate blocks
          // For now, simpler to insert as a paragraph block with an image mark or inline image?
          // Schema does not seem to have 'image' node type in 'nodes' export from schema.ts?
          // Let's check schema.ts... Phase 1 spec didn't mention 'image' block, only 'inline*' content.
          // Standard ProseMirror uses inline image nodes.
          // Wait, schema.ts removed 'image' node from baseSchema?
          // Actually schema.ts removed bullet_list etc but added block. baseSchema has image.
          // let's assume image node exists in baseSchema marks or nodes.
          // schema.ts extends baseSchema.spec.nodes.remove(...).addToEnd(...)
          // baseSchema has 'image' node.

          // However, we can only insert 'block' or 'divider' at top level.
          // To insert an image, we should insert a paragraph block containing the image node.

          const imageType = schema.nodes.image;
          // Check if image node exists in our schema
          if (imageType) {
            const imageNode = imageType.create({
              src: result.url,
              alt: result.name,
              title: null,
            });

            // Create a new paragraph block containing the image
            const block = nodes.block.create({ type: 'paragraph', indent: 0 }, imageNode);
            tr = tr.insert(pos, block);
          }
        } else {
          // Insert link
          const linkType = marks.link;
          if (linkType) {
            const linkMark = linkType.create({ href: result.url, title: null });
            const textNode = schema.text(result.name, [linkMark]);
            // Create a new paragraph block containing the link
            const block = nodes.block.create({ type: 'paragraph', indent: 0 }, textNode);
            tr = tr.insert(pos, block);
          }
        }

        editorView.dispatch(tr);
        props.onAssetUploaded?.();
      } else {
        const errMsg = error();
        if (errMsg) props.onError?.(errMsg);
      }
    }
  };

  const dropPlugin = createDropUploadPlugin({ onFileDrop: handleFileDrop });

  let lastLoadedNodeId: string | undefined = untrack(() => props.nodeId);
  let lastEmittedContent: string = untrack(() => props.content);
  let lastLinkedNodeTitles: NodeTitleMap | undefined = untrack(() => props.linkedNodeTitles);

  const [formatState, setFormatState] = createSignal<FormatState>({
    isBold: false,
    isItalic: false,
    isUnderline: false,
    isStrikethrough: false,
    isCode: false,
    headingLevel: null,
    isBulletList: false,
    isOrderedList: false,
    isTaskList: false,
    isBlockquote: false,
    isCodeBlock: false,
  });

  const [toolbarPosition, setToolbarPosition] = createSignal<{
    top: number;
    bottom: number;
    left: number;
  } | null>(null);

  const localParseMarkdown = (md: string): ProseMirrorNode | null => {
    let processed = rewriteAssetUrls(md, props.assetUrls || {});
    if (props.wsId && props.linkedNodeTitles && Object.keys(props.linkedNodeTitles).length > 0) {
      processed = rewriteInternalLinkTitles(processed, props.linkedNodeTitles, props.wsId);
    }
    return parseMarkdown(processed);
  };

  const localSerializeMarkdown = (doc: ProseMirrorNode): string => {
    const md = serializeToMarkdown(doc);
    return reverseRewriteAssetUrls(md, props.assetUrls || {});
  };

  const updateActiveStates = (editorView: EditorView) => {
    const { state } = editorView;
    const { from, $from, to, empty } = state.selection;

    // Toolbar position
    if (empty || editorMode() === 'markdown') {
      setToolbarPosition(null);
    } else {
      try {
        const start = editorView.coordsAtPos(from);
        const end = editorView.coordsAtPos(to);
        const isSameLine = Math.abs(start.top - end.top) < 20;
        let left = start.left;
        if (isSameLine) {
          left = (start.left + end.right) / 2;
        } else {
          left = start.left + 40;
        }
        setToolbarPosition({
          top: start.top,
          bottom: end.bottom,
          left,
        });
      } catch {
        setToolbarPosition(null);
      }
    }

    // Marks
    const currentMarks = empty ? state.storedMarks || $from.marks() : [];
    const isBold = marks.strong.isInSet(currentMarks) !== undefined || state.doc.rangeHasMark(from, to, marks.strong);
    const isItalic = marks.em.isInSet(currentMarks) !== undefined || state.doc.rangeHasMark(from, to, marks.em);
    const isUnderline =
      marks.underline.isInSet(currentMarks) !== undefined || state.doc.rangeHasMark(from, to, marks.underline);
    const isStrikethrough =
      marks.strikethrough.isInSet(currentMarks) !== undefined || state.doc.rangeHasMark(from, to, marks.strikethrough);
    const isCode = marks.code.isInSet(currentMarks) !== undefined || state.doc.rangeHasMark(from, to, marks.code);

    // Block attributes
    // Find the block node at the cursor/selection
    let blockNode: ProseMirrorNode | null = null;

    // In flat architecture, blocks are always direct children of doc (depth 1)
    if ($from.depth >= 1) {
      blockNode = $from.node(1);
    }

    // If not found (e.g. at start of doc), try nodeAt
    if (!blockNode && from === 0) {
      blockNode = state.doc.nodeAt(0);
    }

    let isBulletList = false;
    let isOrderedList = false;
    let isTaskList = false;
    let isBlockquote = false;
    let isCodeBlock = false;
    let headingLevel: number | null = null;

    // If we have a single block context, use it
    if (blockNode && blockNode.type.name === 'block') {
      const type = blockNode.attrs.type;
      if (type === 'bullet') isBulletList = true;
      if (type === 'number') isOrderedList = true;
      if (type === 'task') isTaskList = true;
      if (type === 'quote') isBlockquote = true;
      if (type === 'code') isCodeBlock = true;
      if (type === 'heading') headingLevel = blockNode.attrs.level;
    }

    setFormatState({
      isBold,
      isItalic,
      isUnderline,
      isStrikethrough,
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
    const editorEl = editorRef();
    if (!editorEl) return;

    const doc = localParseMarkdown(props.content);
    if (!doc) return;
    const state = createEditorState(doc, [slashPlugin, dropPlugin, invalidLinkPlugin]);

    const editorView = new EditorView(editorEl, {
      state,
      editable: () => !props.readOnly,
      nodeViews: {
        block: createBlockNodeView,
      },
      dispatchTransaction(tr) {
        const newState = editorView.state.apply(tr);
        editorView.updateState(newState);

        if (tr.docChanged) {
          const md = localSerializeMarkdown(newState.doc);
          setMarkdownContent(md);
          lastEmittedContent = md;
          props.onChange(md);
        }

        updateActiveStates(editorView);
      },
      handleDOMEvents: {
        click: (_view, event) => {
          const target = event.target as HTMLElement;
          const anchor = target.closest('a');
          if (!anchor) return false;

          const href = anchor.getAttribute('href');
          if (!href) return false;

          const match = href.match(INTERNAL_LINK_URL_PATTERN);
          if (match && match[2] && props.onNavigateToNode) {
            event.preventDefault();
            props.onNavigateToNode(match[2]);
            return true;
          }

          if (href.startsWith('http://') || href.startsWith('https://')) {
            event.preventDefault();
            window.open(href, '_blank', 'noopener,noreferrer');
            return true;
          }

          return false;
        },
      },
    });

    const pmEl = editorEl.querySelector('.ProseMirror') as HTMLElement & { pmView?: EditorView };
    if (pmEl) pmEl.pmView = editorView;

    setView(editorView);
    updateActiveStates(editorView);
    updateInvalidLinkState(editorView, props.linkedNodeTitles || {}, props.wsId);

    const handleScroll = () => updateActiveStates(editorView);
    const scrollContainer = editorEl.closest('[class*="prosemirrorEditor"]') || editorEl;
    scrollContainer.addEventListener('scroll', handleScroll, { passive: true });
    window.addEventListener('scroll', handleScroll, { passive: true });

    onCleanup(() => {
      editorView.destroy();
      scrollContainer.removeEventListener('scroll', handleScroll);
      window.removeEventListener('scroll', handleScroll);
    });
  });

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

          if (nodeChanged || contentChangedExternally) {
            setMarkdownContent(content);
            setEditorMode('wysiwyg');
          }

          const editorView = view();
          if (editorView) {
            const doc = localParseMarkdown(content);
            if (doc) {
              const state = createEditorState(doc, [slashPlugin, dropPlugin, invalidLinkPlugin]);
              editorView.updateState(state);
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
      const doc = localParseMarkdown(markdownContent());
      if (doc) {
        const state = createEditorState(doc, [slashPlugin, dropPlugin, invalidLinkPlugin]);
        editorView.updateState(state);
        updateActiveStates(editorView);
        updateInvalidLinkState(editorView, props.linkedNodeTitles || {}, props.wsId);
      }
    }
    setEditorMode('wysiwyg');
  };

  const switchToMarkdown = () => {
    const editorView = view();
    if (editorView) {
      const md = localSerializeMarkdown(editorView.state.doc);
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
      <EditorToolbar
        formatState={formatState()}
        view={view()}
        position={toolbarPosition()}
        editorElement={editorRef()}
      />

      {/* Editor context menu */}
      <BlockContextMenu view={view()} />

      {/* Drop indicator during drag-and-drop */}
      <EditorDropIndicator view={view()} />

      <div ref={setEditorRef} class={wysiwygClass()} data-testid="wysiwyg-editor" />

      <textarea
        class={markdownClass()}
        value={markdownContent()}
        onInput={(e) => handleMarkdownChange(e.target.value)}
        placeholder={props.placeholder || t('editor.contentPlaceholder') || 'Write markdown...'}
        readOnly={props.readOnly}
        data-testid="markdown-editor"
      />

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
