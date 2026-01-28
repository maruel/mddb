// WYSIWYG markdown editor component using ProseMirror with prosemirror-markdown.

import { createSignal, onMount, onCleanup, createEffect, on, createMemo } from 'solid-js';
import { EditorView } from 'prosemirror-view';
import type { Node as ProseMirrorNode } from 'prosemirror-model';
import { defaultMarkdownParser, schema } from 'prosemirror-markdown';
import { useI18n } from '../../i18n';
import { rewriteAssetUrls, reverseRewriteAssetUrls } from './markdown-utils';
import { markdownSerializer, createEditorState } from './prosemirror-config';
import EditorToolbar, { type FormatState } from './EditorToolbar';
import styles from './Editor.module.css';

interface EditorProps {
  content: string;
  pageId?: string;
  orgId?: string;
  onChange: (markdown: string) => void;
  placeholder?: string;
  readOnly?: boolean;
}

export default function Editor(props: EditorProps) {
  const { t } = useI18n();
  const [editorMode, setEditorMode] = createSignal<'wysiwyg' | 'markdown'>('wysiwyg');
  const [markdownContent, setMarkdownContent] = createSignal(props.content);
  const [view, setView] = createSignal<EditorView | undefined>();
  let editorRef: HTMLDivElement | undefined;

  // Track what we've emitted to distinguish our own changes from external updates
  let lastLoadedPageId: string | undefined = props.pageId;
  let lastEmittedContent: string = props.content;

  // Track active formatting states for toolbar buttons
  const [formatState, setFormatState] = createSignal<FormatState>({
    isBold: false,
    isItalic: false,
    isCode: false,
    headingLevel: null,
    isBulletList: false,
    isOrderedList: false,
    isBlockquote: false,
    isCodeBlock: false,
  });

  // Parse markdown to ProseMirror document, handling asset URLs
  const parseMarkdown = (md: string): ProseMirrorNode | null => {
    const processed = rewriteAssetUrls(md, props.orgId);
    return defaultMarkdownParser.parse(processed);
  };

  // Serialize ProseMirror document to markdown, handling asset URLs
  const serializeMarkdown = (doc: ProseMirrorNode): string => {
    const md = markdownSerializer.serialize(doc);
    return reverseRewriteAssetUrls(md, props.orgId);
  };

  // Update active state signals based on current selection
  const updateActiveStates = (editorView: EditorView) => {
    const { state } = editorView;
    const { from, $from, to, empty } = state.selection;

    // Check marks
    const currentMarks = empty ? state.storedMarks || $from.marks() : [];
    const isBold =
      schema.marks.strong.isInSet(currentMarks) !== undefined || state.doc.rangeHasMark(from, to, schema.marks.strong);
    const isItalic =
      schema.marks.em.isInSet(currentMarks) !== undefined || state.doc.rangeHasMark(from, to, schema.marks.em);
    const isCode =
      schema.marks.code.isInSet(currentMarks) !== undefined || state.doc.rangeHasMark(from, to, schema.marks.code);

    // Check block types
    const node = $from.node($from.depth);
    const headingLevel = node.type === schema.nodes.heading ? (node.attrs.level as number) : null;
    const isCodeBlock = node.type === schema.nodes.code_block;

    // Check list types by walking up ancestors
    let isBulletList = false;
    let isOrderedList = false;
    let isBlockquote = false;
    for (let d = $from.depth; d > 0; d--) {
      const n = $from.node(d);
      if (n.type === schema.nodes.bullet_list) isBulletList = true;
      if (n.type === schema.nodes.ordered_list) isOrderedList = true;
      if (n.type === schema.nodes.blockquote) isBlockquote = true;
    }

    setFormatState({
      isBold,
      isItalic,
      isCode,
      headingLevel,
      isBulletList,
      isOrderedList,
      isBlockquote,
      isCodeBlock,
    });
  };

  onMount(() => {
    if (!editorRef) return;

    const doc = parseMarkdown(props.content);
    if (!doc) return;
    const state = createEditorState(doc);

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

    setView(editorView);
    updateActiveStates(editorView);
  });

  onCleanup(() => {
    view()?.destroy();
  });

  // Sync when page changes or content changes externally
  createEffect(
    on(
      () => [props.pageId, props.content] as const,
      ([pageId, content]) => {
        const pageChanged = pageId !== lastLoadedPageId;
        const contentChangedExternally = content !== lastEmittedContent;

        if (pageChanged || contentChangedExternally) {
          lastLoadedPageId = pageId;
          lastEmittedContent = content;
          setMarkdownContent(content);
          setEditorMode('wysiwyg');

          const editorView = view();
          if (editorView) {
            const doc = parseMarkdown(content);
            if (doc) {
              const state = createEditorState(doc);
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
        const state = createEditorState(doc);
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
    </div>
  );
}
