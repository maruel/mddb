// WYSIWYG markdown editor component using ProseMirror with prosemirror-markdown.

import { createSignal, onMount, onCleanup, createEffect, on, createMemo } from 'solid-js';
import { EditorView } from 'prosemirror-view';
import type { Node as ProseMirrorNode } from 'prosemirror-model';
import { useI18n } from '../../i18n';
import { rewriteAssetUrls, reverseRewriteAssetUrls } from './markdown-utils';
import { nodes, marks, markdownParser, markdownSerializer, createEditorState } from './prosemirror-config';
import { createSlashCommandPlugin, type SlashMenuState } from './slashCommandPlugin';
import SlashCommandMenu from './SlashCommandMenu';
import EditorToolbar, { type FormatState } from './EditorToolbar';
import type { AssetUrlMap } from '../../contexts/EditorContext';
import styles from './Editor.module.css';

interface EditorProps {
  content: string;
  nodeId?: string;
  assetUrls?: AssetUrlMap;
  onChange: (markdown: string) => void;
  placeholder?: string;
  readOnly?: boolean;
}

export default function Editor(props: EditorProps) {
  const { t } = useI18n();
  const [editorMode, setEditorMode] = createSignal<'wysiwyg' | 'markdown'>('wysiwyg');
  const [markdownContent, setMarkdownContent] = createSignal(props.content);
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

  // Track what we've emitted to distinguish our own changes from external updates
  let lastLoadedNodeId: string | undefined = props.nodeId;
  let lastEmittedContent: string = props.content;

  // Track active formatting states for toolbar buttons
  const [formatState, setFormatState] = createSignal<FormatState>({
    isBold: false,
    isItalic: false,
    isCode: false,
    headingLevel: null,
    isBulletList: false,
    isOrderedList: false,
    isTaskList: false,
    isBlockquote: false,
    isCodeBlock: false,
  });

  // Parse markdown to ProseMirror document, handling asset URLs
  const parseMarkdown = (md: string): ProseMirrorNode | null => {
    const processed = rewriteAssetUrls(md, props.assetUrls || {});
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

    // Check marks
    const currentMarks = empty ? state.storedMarks || $from.marks() : [];
    const isBold = marks.strong.isInSet(currentMarks) !== undefined || state.doc.rangeHasMark(from, to, marks.strong);
    const isItalic = marks.em.isInSet(currentMarks) !== undefined || state.doc.rangeHasMark(from, to, marks.em);
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

    setFormatState({
      isBold,
      isItalic,
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
    const state = createEditorState(doc, [slashPlugin]);

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
    });

    // Store view reference on DOM element for checkbox plugin access
    const pmEl = editorRef.querySelector('.ProseMirror') as HTMLElement & { pmView?: EditorView };
    if (pmEl) {
      pmEl.pmView = editorView;
    }

    setView(editorView);
    updateActiveStates(editorView);
  });

  onCleanup(() => {
    view()?.destroy();
  });

  // Sync when node changes or content changes externally
  createEffect(
    on(
      () => [props.nodeId, props.content] as const,
      ([nodeId, content]) => {
        const nodeChanged = nodeId !== lastLoadedNodeId;
        const contentChangedExternally = content !== lastEmittedContent;

        if (nodeChanged || contentChangedExternally) {
          lastLoadedNodeId = nodeId;
          lastEmittedContent = content;
          setMarkdownContent(content);
          setEditorMode('wysiwyg');

          const editorView = view();
          if (editorView) {
            const doc = parseMarkdown(content);
            if (doc) {
              const state = createEditorState(doc, [slashPlugin]);
              editorView.updateState(state);
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
        const state = createEditorState(doc, [slashPlugin]);
        editorView.updateState(state);
        updateActiveStates(editorView);
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

  const wysiwygClass = createMemo(() =>
    editorMode() === 'wysiwyg' ? styles.prosemirrorEditor : `${styles.prosemirrorEditor} ${styles.hidden}`
  );

  const markdownClass = createMemo(() =>
    editorMode() === 'markdown' ? styles.markdownEditor : `${styles.markdownEditor} ${styles.hidden}`
  );

  return (
    <div class={styles.editorContainer}>
      <EditorToolbar
        editorMode={editorMode()}
        formatState={formatState()}
        view={view()}
        onSwitchToWysiwyg={switchToWysiwyg}
        onSwitchToMarkdown={switchToMarkdown}
      />

      <div ref={editorRef} class={wysiwygClass()} data-testid="wysiwyg-editor" />

      <textarea
        class={markdownClass()}
        value={markdownContent()}
        onInput={(e) => handleMarkdownChange(e.target.value)}
        placeholder={props.placeholder || t('editor.contentPlaceholder') || 'Write markdown...'}
        readOnly={props.readOnly}
        data-testid="markdown-editor"
      />

      {(() => {
        const v = view();
        return v ? <SlashCommandMenu view={v} state={slashMenuState()} /> : null;
      })()}
    </div>
  );
}
