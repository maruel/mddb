// WYSIWYG markdown editor component using ProseMirror with prosemirror-markdown.

import { createSignal, onMount, onCleanup, Show, createEffect, on } from 'solid-js';
import { EditorState, type Transaction } from 'prosemirror-state';
import { EditorView } from 'prosemirror-view';
import type { Node as ProseMirrorNode } from 'prosemirror-model';
import { defaultMarkdownParser, defaultMarkdownSerializer, schema, MarkdownSerializer } from 'prosemirror-markdown';
import { history, undo, redo } from 'prosemirror-history';
import { keymap } from 'prosemirror-keymap';
import { baseKeymap, toggleMark, setBlockType, wrapIn, lift } from 'prosemirror-commands';
import { dropCursor } from 'prosemirror-dropcursor';
import { gapCursor } from 'prosemirror-gapcursor';
import { wrapInList, splitListItem, liftListItem, sinkListItem } from 'prosemirror-schema-list';
import {
  inputRules,
  wrappingInputRule,
  textblockTypeInputRule,
  smartQuotes,
  emDash,
  ellipsis,
} from 'prosemirror-inputrules';
import { useI18n } from '../../i18n';
import { rewriteAssetUrls, reverseRewriteAssetUrls } from './markdown-utils';
import styles from './Editor.module.css';

// Custom markdown serializer that uses "-" for bullet lists instead of "*"
const markdownSerializer = new MarkdownSerializer(
  {
    ...defaultMarkdownSerializer.nodes,
    // Override bullet_list to use "-" instead of "*"
    bullet_list(state, node) {
      state.renderList(node, '  ', () => '- ');
    },
  },
  defaultMarkdownSerializer.marks
);

interface EditorProps {
  content: string;
  pageId?: string; // Used to detect page changes and reset editor state
  orgId?: string;
  onChange: (markdown: string) => void;
  placeholder?: string;
  readOnly?: boolean;
}

// Build input rules for markdown-like shortcuts
function buildInputRules() {
  const rules = smartQuotes.concat(ellipsis, emDash);

  // Blockquote: > at start of line
  rules.push(wrappingInputRule(/^\s*>\s$/, schema.nodes.blockquote));

  // Bullet list: - or * at start of line
  rules.push(wrappingInputRule(/^\s*([-*])\s$/, schema.nodes.bullet_list));

  // Ordered list: 1. at start of line
  rules.push(
    wrappingInputRule(
      /^(\d+)\.\s$/,
      schema.nodes.ordered_list,
      (match) => ({ order: +(match[1] ?? 1) }),
      (match, node) => node.childCount + (node.attrs.order as number) === +(match[1] ?? 1)
    )
  );

  // Code block: ``` at start of line
  rules.push(textblockTypeInputRule(/^```$/, schema.nodes.code_block));

  // Headings: # ## ### etc at start of line
  for (let i = 1; i <= 6; i++) {
    const pattern = new RegExp(`^(#{${i}})\\s$`);
    rules.push(textblockTypeInputRule(pattern, schema.nodes.heading, { level: i }));
  }

  return inputRules({ rules });
}

// Build keymap for list operations and formatting
function buildKeymap() {
  const keys: { [key: string]: (state: EditorState, dispatch?: (tr: Transaction) => void) => boolean } = {};

  // History
  keys['Mod-z'] = undo;
  keys['Mod-y'] = redo;
  keys['Mod-Shift-z'] = redo;

  // Formatting marks
  keys['Mod-b'] = toggleMark(schema.marks.strong);
  keys['Mod-i'] = toggleMark(schema.marks.em);
  keys['Mod-`'] = toggleMark(schema.marks.code);

  // List operations
  keys['Enter'] = splitListItem(schema.nodes.list_item);
  keys['Tab'] = sinkListItem(schema.nodes.list_item);
  keys['Shift-Tab'] = liftListItem(schema.nodes.list_item);

  return keymap(keys);
}

export default function Editor(props: EditorProps) {
  const { t } = useI18n();
  const [editorMode, setEditorMode] = createSignal<'wysiwyg' | 'markdown'>('wysiwyg');
  const [markdownContent, setMarkdownContent] = createSignal(props.content);
  let editorRef: HTMLDivElement | undefined;
  let view: EditorView | undefined;

  // Track what we've emitted to distinguish our own changes from external updates
  let lastLoadedPageId: string | undefined = props.pageId;
  let lastEmittedContent: string = props.content;

  // Track active formatting states for toolbar buttons
  const [isBold, setIsBold] = createSignal(false);
  const [isItalic, setIsItalic] = createSignal(false);
  const [isCode, setIsCode] = createSignal(false);
  const [headingLevel, setHeadingLevel] = createSignal<number | null>(null);
  const [isBulletList, setIsBulletList] = createSignal(false);
  const [isOrderedList, setIsOrderedList] = createSignal(false);
  const [isBlockquote, setIsBlockquote] = createSignal(false);
  const [isCodeBlock, setIsCodeBlock] = createSignal(false);

  // Parse markdown to ProseMirror document, handling asset URLs
  const parseMarkdown = (md: string) => {
    const processed = rewriteAssetUrls(md, props.orgId);
    return defaultMarkdownParser.parse(processed);
  };

  // Serialize ProseMirror document to markdown, handling asset URLs
  const serializeMarkdown = (doc: ProseMirrorNode): string => {
    const md = markdownSerializer.serialize(doc);
    return reverseRewriteAssetUrls(md, props.orgId);
  };

  // Update active state signals based on current selection
  const updateActiveStates = () => {
    if (!view) return;
    const { state } = view;
    const { from, $from, to, empty } = state.selection;

    // Check marks
    const currentMarks = empty ? state.storedMarks || $from.marks() : [];
    setIsBold(
      schema.marks.strong.isInSet(currentMarks) !== undefined || state.doc.rangeHasMark(from, to, schema.marks.strong)
    );
    setIsItalic(
      schema.marks.em.isInSet(currentMarks) !== undefined || state.doc.rangeHasMark(from, to, schema.marks.em)
    );
    setIsCode(
      schema.marks.code.isInSet(currentMarks) !== undefined || state.doc.rangeHasMark(from, to, schema.marks.code)
    );

    // Check block types
    const node = $from.node($from.depth);

    if (node.type === schema.nodes.heading) {
      setHeadingLevel(node.attrs.level as number);
    } else {
      setHeadingLevel(null);
    }

    setIsCodeBlock(node.type === schema.nodes.code_block);

    // Check list types by walking up ancestors
    let inBulletList = false;
    let inOrderedList = false;
    let inBlockquote = false;
    for (let d = $from.depth; d > 0; d--) {
      const n = $from.node(d);
      if (n.type === schema.nodes.bullet_list) inBulletList = true;
      if (n.type === schema.nodes.ordered_list) inOrderedList = true;
      if (n.type === schema.nodes.blockquote) inBlockquote = true;
    }
    setIsBulletList(inBulletList);
    setIsOrderedList(inOrderedList);
    setIsBlockquote(inBlockquote);
  };

  // Create editor state with plugins
  const createEditorState = (doc: ProseMirrorNode) => {
    return EditorState.create({
      doc,
      plugins: [
        buildInputRules(),
        buildKeymap(),
        keymap(baseKeymap),
        history(),
        dropCursor({ color: 'var(--c-primary)', width: 2 }),
        gapCursor(),
      ],
    });
  };

  onMount(() => {
    if (!editorRef) return;

    const doc = parseMarkdown(props.content);
    if (!doc) return;
    const state = createEditorState(doc);

    view = new EditorView(editorRef, {
      state,
      editable: () => !props.readOnly,
      dispatchTransaction(tr) {
        if (!view) return;
        const newState = view.state.apply(tr);
        view.updateState(newState);

        if (tr.docChanged) {
          const md = serializeMarkdown(newState.doc);
          setMarkdownContent(md);
          lastEmittedContent = md;
          props.onChange(md);
        }

        // Update active states on any transaction (including selection changes)
        updateActiveStates();
      },
    });

    // Initial state update
    updateActiveStates();
  });

  onCleanup(() => {
    view?.destroy();
  });

  // Sync when page changes (detected via pageId prop changing)
  // Also track content to handle cases where content loads after pageId
  // Use `on` with `defer: true` to skip the initial run (handled by onMount)
  //
  // We track lastEmittedContent to distinguish between:
  // - Content changed by us (via onChange) → don't reset (content === lastEmittedContent)
  // - Content changed externally (page navigation) → do reset (content !== lastEmittedContent)
  createEffect(
    on(
      () => [props.pageId, props.content] as const,
      ([pageId, content]) => {
        const pageChanged = pageId !== lastLoadedPageId;
        // Content changed externally if it differs from what we last emitted
        const contentChangedExternally = content !== lastEmittedContent;

        if (pageChanged || contentChangedExternally) {
          lastLoadedPageId = pageId;
          lastEmittedContent = content;
          setMarkdownContent(content);
          setEditorMode('wysiwyg');
          if (view) {
            const doc = parseMarkdown(content);
            if (doc) {
              const state = createEditorState(doc);
              view.updateState(state);
            }
          }
        }
      },
      { defer: true }
    )
  );

  // Handle markdown textarea changes
  const handleMarkdownChange = (value: string) => {
    setMarkdownContent(value);
    lastEmittedContent = value;
    props.onChange(value);
  };

  // Switch from markdown mode to WYSIWYG
  const switchToWysiwyg = () => {
    if (view) {
      const doc = parseMarkdown(markdownContent());
      if (doc) {
        const state = createEditorState(doc);
        view.updateState(state);
        updateActiveStates();
      }
    }
    setEditorMode('wysiwyg');
  };

  // Switch from WYSIWYG to markdown
  const switchToMarkdown = () => {
    if (view) {
      const md = serializeMarkdown(view.state.doc);
      setMarkdownContent(md);
    }
    setEditorMode('markdown');
  };

  // Toolbar command helpers
  const toggleBold = () => {
    if (!view) return;
    toggleMark(schema.marks.strong)(view.state, view.dispatch);
    view.focus();
  };

  const toggleItalic = () => {
    if (!view) return;
    toggleMark(schema.marks.em)(view.state, view.dispatch);
    view.focus();
  };

  const toggleCodeMark = () => {
    if (!view) return;
    toggleMark(schema.marks.code)(view.state, view.dispatch);
    view.focus();
  };

  const setHeading = (level: number) => {
    if (!view) return;
    const { state, dispatch } = view;

    // If already at this heading level, convert to paragraph
    if (headingLevel() === level) {
      setBlockType(schema.nodes.paragraph)(state, dispatch);
    } else {
      setBlockType(schema.nodes.heading, { level })(state, dispatch);
    }
    view.focus();
  };

  const toggleBulletListCmd = () => {
    if (!view) return;
    const { state, dispatch } = view;
    if (isBulletList()) {
      lift(state, dispatch);
    } else {
      wrapInList(schema.nodes.bullet_list)(state, dispatch);
    }
    view.focus();
  };

  const toggleOrderedListCmd = () => {
    if (!view) return;
    const { state, dispatch } = view;
    if (isOrderedList()) {
      lift(state, dispatch);
    } else {
      wrapInList(schema.nodes.ordered_list)(state, dispatch);
    }
    view.focus();
  };

  const toggleBlockquoteCmd = () => {
    if (!view) return;
    const { state, dispatch } = view;
    if (isBlockquote()) {
      lift(state, dispatch);
    } else {
      wrapIn(schema.nodes.blockquote)(state, dispatch);
    }
    view.focus();
  };

  const toggleCodeBlockCmd = () => {
    if (!view) return;
    const { state, dispatch } = view;
    if (isCodeBlock()) {
      setBlockType(schema.nodes.paragraph)(state, dispatch);
    } else {
      setBlockType(schema.nodes.code_block)(state, dispatch);
    }
    view.focus();
  };

  const modeButtonClass = (isActive: boolean) =>
    isActive ? `${styles.modeButton} ${styles.active}` : styles.modeButton;

  const formatButtonClass = (isActive: boolean) =>
    isActive ? `${styles.formatButton} ${styles.isActive}` : styles.formatButton;

  // Use CSS classes to show/hide editors instead of conditional rendering
  const wysiwygClass = () =>
    editorMode() === 'wysiwyg' ? styles.prosemirrorEditor : `${styles.prosemirrorEditor} ${styles.hidden}`;

  const markdownClass = () =>
    editorMode() === 'markdown' ? styles.markdownEditor : `${styles.markdownEditor} ${styles.hidden}`;

  return (
    <div class={styles.editorContainer}>
      <div class={styles.editorToolbar}>
        <div class={styles.modeToggle}>
          <button
            class={modeButtonClass(editorMode() === 'wysiwyg')}
            onClick={switchToWysiwyg}
            title={t('editor.wysiwygMode') || 'Visual'}
            data-testid="editor-mode-visual"
          >
            {t('editor.wysiwygMode') || 'Visual'}
          </button>
          <button
            class={modeButtonClass(editorMode() === 'markdown')}
            onClick={switchToMarkdown}
            title={t('editor.markdownMode') || 'Markdown'}
            data-testid="editor-mode-markdown"
          >
            {t('editor.markdownMode') || 'Markdown'}
          </button>
        </div>
        <Show when={editorMode() === 'wysiwyg'}>
          <div class={styles.formattingButtons}>
            <button class={formatButtonClass(isBold())} onClick={toggleBold} title="Bold (Ctrl+B)">
              B
            </button>
            <button class={formatButtonClass(isItalic())} onClick={toggleItalic} title="Italic (Ctrl+I)">
              I
            </button>
            <button class={formatButtonClass(isCode())} onClick={toggleCodeMark} title="Code (Ctrl+`)">
              {'</>'}
            </button>
            <span class={styles.separator} />
            <button class={formatButtonClass(headingLevel() === 1)} onClick={() => setHeading(1)} title="Heading 1">
              H1
            </button>
            <button class={formatButtonClass(headingLevel() === 2)} onClick={() => setHeading(2)} title="Heading 2">
              H2
            </button>
            <button class={formatButtonClass(headingLevel() === 3)} onClick={() => setHeading(3)} title="Heading 3">
              H3
            </button>
            <span class={styles.separator} />
            <button class={formatButtonClass(isBulletList())} onClick={toggleBulletListCmd} title="Bullet List">
              •
            </button>
            <button class={formatButtonClass(isOrderedList())} onClick={toggleOrderedListCmd} title="Numbered List">
              1.
            </button>
            <button class={formatButtonClass(isBlockquote())} onClick={toggleBlockquoteCmd} title="Quote">
              "
            </button>
            <button class={formatButtonClass(isCodeBlock())} onClick={toggleCodeBlockCmd} title="Code Block">
              {'{ }'}
            </button>
          </div>
        </Show>
      </div>

      {/* Always render both editors, use CSS to show/hide */}
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
