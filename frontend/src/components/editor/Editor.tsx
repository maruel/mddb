// ProseMirror editor with mapped asynchronous insertions and guarded mode changes.

import { createSignal, onMount, onCleanup, createEffect, on, createMemo, Show, untrack } from "solid-js";
import { EditorView } from "prosemirror-view";
import type { Node as ProseMirrorNode } from "prosemirror-model";
import { useI18n } from "../../i18n";
import {
  rewriteAssetUrls,
  reverseRewriteAssetUrls,
  rewriteInternalLinkTitles,
  type NodeTitleMap,
} from "./markdown-utils";
import { nodes, marks, createEditorState, schema } from "./prosemirror-config";
import { parseMarkdown } from "./markdown-parser";
import { serializeToMarkdown } from "./markdown-serializer";
import { transformPastedHTML } from "./dom-parser";
import { createBlockNodeView } from "./BlockNodeView";
import { isTaskCheckboxClick, taskBlockPos, toggleTaskBlock } from "./blockCommands";
import { createSlashCommandPlugin, type SlashMenuState } from "./slashCommandPlugin";
import { createDropUploadPlugin } from "./dropUploadPlugin";
import { createInvalidLinkPlugin, updateInvalidLinkState, INTERNAL_LINK_URL_PATTERN } from "./invalidLinkPlugin";
import { useAssetUpload, isImageMimeType } from "./useAssetUpload";
import { BlockContextMenu } from "./BlockContextMenu";
import { EditorDropIndicator } from "./EditorDropIndicator";
import SlashCommandMenu from "./SlashCommandMenu";
import EditorToolbar, { type FormatState } from "./EditorToolbar";
import type { AssetUrlMap, RunAsyncEdit, SavedEditContinuation } from "../../contexts/EditorContext";
import styles from "./Editor.module.css";

interface EditorProps {
  content: string;
  nodeId?: string;
  assetUrls?: AssetUrlMap;
  linkedNodeTitles?: NodeTitleMap;
  onChange: (markdown: string) => void;
  placeholder?: string;
  readOnly?: boolean;
  wsId?: string;
  sessionID?: string;
  onAssetUploaded?: () => void;
  runAsyncEdit: RunAsyncEdit;
  asyncEditPending: boolean;
  onNavigateToNode?: (nodeId: string) => void;
}

export type AsyncInsertion = (
  pos: number,
  edit: (insert: (node: ProseMirrorNode) => void) => Promise<void>,
  afterSave?: SavedEditContinuation,
) => Promise<void>;

export default function Editor(props: EditorProps) {
  const { t } = useI18n();
  const [editorMode, setEditorMode] = createSignal<"wysiwyg" | "markdown">("wysiwyg");
  const [markdownContent, setMarkdownContent] = createSignal(untrack(() => props.content));
  const [view, setView] = createSignal<EditorView | undefined>();
  const [slashMenuState, setSlashMenuState] = createSignal<SlashMenuState>({
    active: false,
    query: "",
    triggerPos: 0,
    position: { top: 0, left: 0 },
  });
  const [editorRef, setEditorRef] = createSignal<HTMLDivElement>();

  // Create slash command plugin with callback
  const slashPlugin = createSlashCommandPlugin(setSlashMenuState);

  // Create invalid link decoration plugin
  const invalidLinkPlugin = createInvalidLinkPlugin();

  // Drop anchors follow ordinary edits until their asynchronous insertions commit.
  const insertions = new Set<{ pos: number }>();

  // Every asynchronous body writer shares admission, mapped anchors, and retirement.
  const insertAsync: AsyncInsertion = (pos, edit, afterSave) =>
    props.runAsyncEdit(
      () =>
        untrack(async () => {
          const initial = view();
          if (!initial) return false;
          const session = props.sessionID,
            nodeId = props.nodeId,
            wsId = props.wsId;
          const current = () =>
            !initial.isDestroyed &&
            view() === initial &&
            props.sessionID === session &&
            props.nodeId === nodeId &&
            props.wsId === wsId;
          const insertion = { pos };
          insertions.add(insertion);
          try {
            await edit((node) => {
              if (!current() || props.readOnly) throw new Error(t("editor.asyncEditFailed"));
              initial.dispatch(initial.state.tr.insert(insertion.pos, node));
            });
            return current();
          } finally {
            insertions.delete(insertion);
          }
        }),
      afterSave,
    );

  const handleFileDrop = (files: File[], pos: number) =>
    insertAsync(pos, (insert) =>
      untrack(async () => {
        if (!props.wsId || !props.nodeId) throw new Error(t("editor.asyncEditFailed"));
        const { uploadFile, error } = useAssetUpload({
          wsId: props.wsId,
          nodeId: props.nodeId,
          sessionID: props.sessionID,
        });
        for (const file of files) {
          const result = await uploadFile(file);
          if (!result) throw new Error(error() ?? t("errors.failedToSave"));
          const inline = isImageMimeType(result.mimeType)
            ? schema.node("image", { src: result.url, alt: result.name, title: null })
            : schema.text(result.name, [schema.mark("link", { href: result.url, title: null })]);
          insert(nodes.block.create({ type: "paragraph", indent: 0 }, inline));
          props.onAssetUploaded?.();
        }
      }),
    );

  const dropPlugin = createDropUploadPlugin({
    onFileDrop: (file, pos) => {
      void handleFileDrop(file, pos).catch((error: unknown) => console.error("File upload failed:", error));
    },
  });

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

  const resolveMarkdown = (md: string, titles: NodeTitleMap | undefined): string => {
    let processed = rewriteAssetUrls(md, props.assetUrls || {});
    if (props.wsId && titles) {
      processed = rewriteInternalLinkTitles(processed, titles, props.wsId);
    }
    return processed;
  };

  const localSerializeMarkdown = (doc: ProseMirrorNode): string => {
    const md = serializeToMarkdown(doc);
    return reverseRewriteAssetUrls(md, props.assetUrls || {});
  };

  const updateActiveStates = (editorView: EditorView) => {
    const { state } = editorView;
    const { from, $from, to, empty } = state.selection;

    // Toolbar position
    if (empty || editorMode() === "markdown") {
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
    if (blockNode && blockNode.type.name === "block") {
      const type: unknown = blockNode.attrs.type;
      if (type === "bullet") isBulletList = true;
      if (type === "number") isOrderedList = true;
      if (type === "task") isTaskList = true;
      if (type === "quote") isBlockquote = true;
      if (type === "code") isCodeBlock = true;
      if (type === "heading") headingLevel = blockNode.attrs.level as number | null;
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

    const doc = parseMarkdown(resolveMarkdown(props.content, props.linkedNodeTitles));
    const state = createEditorState(doc, [slashPlugin, dropPlugin, invalidLinkPlugin]);

    const editorView = new EditorView(editorEl, {
      state,
      editable: () => !props.readOnly,
      transformPastedHTML(html) {
        return transformPastedHTML(html);
      },
      clipboardTextSerializer(slice) {
        return localSerializeMarkdown(schema.node("doc", null, slice.content));
      },
      handlePaste(_view, event) {
        const html = event.clipboardData?.getData("text/html");
        if (html) return false; // Let transformPastedHTML handle it

        const text = event.clipboardData?.getData("text/plain");
        if (!text) return false;

        // Heuristic: if it has multiple lines or markdown markers, parse as markdown
        const hasMarkers = /^[#>\-*\d+]|[`[*_]/.test(text) || text.includes("\n");
        if (hasMarkers) {
          const parsed = parseMarkdown(resolveMarkdown(text, props.linkedNodeTitles));
          const slice = parsed.slice(0, parsed.content.size);
          editorView.dispatch(editorView.state.tr.replaceSelection(slice));
          return true;
        }
        return false;
      },
      nodeViews: {
        block: createBlockNodeView,
      },
      dispatchTransaction(tr) {
        if (props.readOnly && tr.docChanged) return;
        const applied = editorView.state.applyTransaction(tr);
        const docChanged = applied.transactions.some((transaction) => transaction.docChanged);
        // A selection transaction can also cause an appended document change.
        if (props.readOnly && docChanged) return;
        for (const transaction of applied.transactions) {
          // Association after the boundary also resolves deleted drop targets
          // and keeps multiple inserted files in their original order.
          for (const insertion of insertions) insertion.pos = transaction.mapping.map(insertion.pos, 1);
        }
        const newState = applied.state;
        editorView.updateState(newState);

        if (docChanged) {
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

          // Handle checkbox toggle: clicks on the ::before pseudo-element area (left padding)
          const taskEl = target.closest<HTMLElement>(".block-task");
          if (taskEl && isTaskCheckboxClick(event, taskEl)) {
            event.preventDefault();
            toggleTaskBlock(taskBlockPos(editorView, taskEl))(editorView.state, editorView.dispatch);
            return true;
          }

          const anchor = target.closest("a");
          if (!anchor) return false;

          const href = anchor.getAttribute("href");
          if (!href) return false;

          const match = href.match(INTERNAL_LINK_URL_PATTERN);
          if (match && match[2] && props.onNavigateToNode) {
            event.preventDefault();
            props.onNavigateToNode(match[2]);
            return true;
          }

          if (href.startsWith("http://") || href.startsWith("https://")) {
            event.preventDefault();
            window.open(href, "_blank", "noopener,noreferrer");
            return true;
          }

          return false;
        },
      },
    });

    const pmEl = editorEl.querySelector<HTMLElement & { pmView?: EditorView }>(".ProseMirror");
    if (pmEl) pmEl.pmView = editorView;

    setView(editorView);
    updateActiveStates(editorView);
    updateInvalidLinkState(editorView, props.linkedNodeTitles || {}, props.wsId);

    const handleScroll = () => updateActiveStates(editorView);
    const scrollContainer = editorEl.closest('[class*="prosemirrorEditor"]') || editorEl;
    scrollContainer.addEventListener("scroll", handleScroll, { passive: true });
    window.addEventListener("scroll", handleScroll, { passive: true });

    onCleanup(() => {
      insertions.clear();
      editorView.destroy();
      scrollContainer.removeEventListener("scroll", handleScroll);
      window.removeEventListener("scroll", handleScroll);
    });
  });

  createEffect(() => {
    const editable = !props.readOnly;
    view()?.setProps({ editable: () => editable });
  });

  createEffect(
    on(
      () => [props.nodeId, props.content, props.linkedNodeTitles, props.asyncEditPending] as const,
      ([nodeId, content, linkedNodeTitles]) => {
        const nodeChanged = nodeId !== lastLoadedNodeId;
        if (nodeChanged) insertions.clear();
        // A same-node refresh must not replace the representation or bypass
        // transaction mapping while an admitted insertion still owns it.
        if (!nodeChanged && props.asyncEditPending) return;
        const contentChangedExternally = content !== lastEmittedContent;
        const titlesChanged = linkedNodeTitles !== lastLinkedNodeTitles;
        // Compare the same Markdown under each title map. Reparsing the live
        // document would discard transient typing states such as trailing spaces.
        const titlesChangeContent =
          titlesChanged &&
          resolveMarkdown(content, linkedNodeTitles) !== resolveMarkdown(content, lastLinkedNodeTitles);

        if (nodeChanged || contentChangedExternally || titlesChanged) {
          lastLoadedNodeId = nodeId;
          lastEmittedContent = content;
          lastLinkedNodeTitles = linkedNodeTitles;

          if (nodeChanged || contentChangedExternally) {
            setMarkdownContent(content);
            setEditorMode("wysiwyg");
          }

          const editorView = view();
          if (editorView) {
            if (nodeChanged || contentChangedExternally || titlesChangeContent) {
              const doc = parseMarkdown(resolveMarkdown(content, linkedNodeTitles));
              if (nodeChanged || !doc.eq(editorView.state.doc)) {
                const state = createEditorState(doc, [slashPlugin, dropPlugin, invalidLinkPlugin]);
                editorView.updateState(state);
              }
            }
            updateInvalidLinkState(editorView, linkedNodeTitles || {}, props.wsId);
          }
        }
      },
      { defer: true },
    ),
  );

  const handleMarkdownChange = (value: string) => {
    if (props.readOnly) return;
    setMarkdownContent(value);
    lastEmittedContent = value;
    props.onChange(value);
  };

  const modeLocked = () => props.readOnly || props.asyncEditPending;
  // All user mode changes, including document-level shortcuts, go through here.
  // Pending insertions must keep their representation; rebuilding a locked or
  // already-active editor would discard its undo history.
  const switchMode = (mode: "wysiwyg" | "markdown") => {
    if (modeLocked() || editorMode() === mode) return;
    const editorView = view();
    if (editorView) {
      if (mode === "wysiwyg") {
        const doc = parseMarkdown(resolveMarkdown(markdownContent(), props.linkedNodeTitles));
        const state = createEditorState(doc, [slashPlugin, dropPlugin, invalidLinkPlugin]);
        editorView.updateState(state);
        updateActiveStates(editorView);
        updateInvalidLinkState(editorView, props.linkedNodeTitles || {}, props.wsId);
      } else {
        setMarkdownContent(localSerializeMarkdown(editorView.state.doc));
      }
    }
    setEditorMode(mode);
  };

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.ctrlKey && e.shiftKey && e.key.toLowerCase() === "m") {
      e.preventDefault();
      switchMode(editorMode() === "wysiwyg" ? "markdown" : "wysiwyg");
    }
  };

  onMount(() => {
    document.addEventListener("keydown", handleKeyDown);
  });
  onCleanup(() => {
    document.removeEventListener("keydown", handleKeyDown);
  });

  const wysiwygClass = createMemo(() =>
    editorMode() === "wysiwyg" ? styles.prosemirrorEditor : `${styles.prosemirrorEditor} ${styles.hidden}`,
  );

  const markdownClass = createMemo(() =>
    editorMode() === "markdown" ? styles.markdownEditor : `${styles.markdownEditor} ${styles.hidden}`,
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
        placeholder={props.placeholder || t("editor.contentPlaceholder") || "Write markdown..."}
        readOnly={props.readOnly}
        data-testid="markdown-editor"
      />

      <div class={styles.modeIndicator}>
        <button
          class={editorMode() === "wysiwyg" ? styles.modeIndicatorActive : undefined}
          onClick={() => switchMode("wysiwyg")}
          disabled={modeLocked()}
          title="Visual mode (Ctrl+Shift+M)"
          data-testid="editor-mode-visual"
        >
          Visual
        </button>
        <button
          class={editorMode() === "markdown" ? styles.modeIndicatorActive : undefined}
          onClick={() => switchMode("markdown")}
          disabled={modeLocked()}
          title="Markdown mode (Ctrl+Shift+M)"
          data-testid="editor-mode-markdown"
        >
          MD
        </button>
      </div>

      <Show when={view()}>
        {(v) => (
          <SlashCommandMenu view={v()} state={slashMenuState()} nodeId={props.nodeId} insertAsync={insertAsync} />
        )}
      </Show>
    </div>
  );
}
