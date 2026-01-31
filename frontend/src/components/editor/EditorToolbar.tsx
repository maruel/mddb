// Floating editor toolbar with formatting buttons (appears on text selection).

import { Show, createSignal, createEffect, on } from 'solid-js';
import type { EditorView } from 'prosemirror-view';
import type { Node as PMNode } from 'prosemirror-model';
import { Selection, TextSelection, AllSelection } from 'prosemirror-state';
import { toggleMark, setBlockType, wrapIn, lift } from 'prosemirror-commands';
import { wrapInList } from 'prosemirror-schema-list';
import { nodes, marks } from './prosemirror-config';
import styles from './Editor.module.css';

export interface FormatState {
  isBold: boolean;
  isItalic: boolean;
  isUnderline: boolean;
  isStrikethrough: boolean;
  isCode: boolean;
  headingLevel: number | null;
  isBulletList: boolean;
  isOrderedList: boolean;
  isTaskList: boolean;
  isBlockquote: boolean;
  isCodeBlock: boolean;
}

interface EditorToolbarProps {
  formatState: FormatState;
  view: EditorView | undefined;
  position?: { top: number; bottom: number; left: number } | null;
  editorElement?: HTMLDivElement;
}

export default function EditorToolbar(props: EditorToolbarProps) {
  let toolbarRef: HTMLDivElement | undefined;
  const [above, setAbove] = createSignal(false);
  const [clampedLeft, setClampedLeft] = createSignal<number | null>(null);

  // Check if toolbar fits below selection (flip above if not) and clamp horizontally
  createEffect(
    on(
      () => props.position,
      () => {
        if (!props.position || !toolbarRef) {
          setAbove(false);
          setClampedLeft(null);
          return;
        }
        // Use requestAnimationFrame to measure after render
        requestAnimationFrame(() => {
          if (!toolbarRef || !props.position) return;
          const rect = toolbarRef.getBoundingClientRect();

          // Vertical: flip above if would overflow bottom
          const wouldOverflow = props.position.bottom + rect.height + 8 > window.innerHeight;
          setAbove(wouldOverflow);

          // Horizontal: clamp so toolbar stays within viewport
          // The toolbar is centered (translateX(-50%)), so we need halfWidth for bounds
          const halfWidth = rect.width / 2;

          // Use the editor element bounds (accounts for sidebar)
          const editorRect = props.editorElement?.getBoundingClientRect();
          const editorLeft = editorRect?.left ?? 0;
          const editorRight = editorRect?.right ?? window.innerWidth;

          const minLeft = editorLeft + halfWidth;
          const maxLeft = editorRight - halfWidth;
          const left = props.position.left;

          if (left < minLeft) {
            setClampedLeft(minLeft);
          } else if (left > maxLeft) {
            setClampedLeft(maxLeft);
          } else {
            setClampedLeft(null); // No clamping needed
          }
        });
      }
    )
  );

  const formatButtonClass = (isActive: boolean) =>
    isActive ? `${styles.formatButton} ${styles.isActive}` : styles.formatButton;

  // Get current list state from editor (not stale props) for use in click handlers
  const getCurrentListState = () => {
    if (!props.view) return { isBulletList: false, isOrderedList: false, isTaskList: false };
    const { state } = props.view;
    const { from, to, $from } = state.selection;

    let isBulletList = false;
    let isOrderedList = false;
    let isTaskList = false;

    // Check ancestors
    for (let d = $from.depth; d > 0; d--) {
      const n = $from.node(d);
      if (n.type === nodes.bullet_list) isBulletList = true;
      if (n.type === nodes.ordered_list) isOrderedList = true;
      if (n.type === nodes.list_item && n.attrs.checked !== null) isTaskList = true;
    }

    // Also check nodes in selection range (for Ctrl+A selection)
    if (!isBulletList && !isOrderedList && !isTaskList) {
      state.doc.nodesBetween(from, to, (n) => {
        if (n.type === nodes.bullet_list) isBulletList = true;
        if (n.type === nodes.ordered_list) isOrderedList = true;
        if (n.type === nodes.list_item && n.attrs.checked !== null) isTaskList = true;
      });
    }

    // Task lists are bullet lists with checked attrs - mutually exclusive
    if (isTaskList) isBulletList = false;

    return { isBulletList, isOrderedList, isTaskList };
  };

  const toggleBold = () => {
    if (!props.view) return;
    toggleMark(marks.strong)(props.view.state, props.view.dispatch);
    props.view.focus();
  };

  const toggleItalic = () => {
    if (!props.view) return;
    toggleMark(marks.em)(props.view.state, props.view.dispatch);
    props.view.focus();
  };

  const toggleUnderline = () => {
    if (!props.view) return;
    toggleMark(marks.underline)(props.view.state, props.view.dispatch);
    props.view.focus();
  };

  const toggleStrikethrough = () => {
    if (!props.view) return;
    toggleMark(marks.strikethrough)(props.view.state, props.view.dispatch);
    props.view.focus();
  };

  const toggleCode = () => {
    if (!props.view) return;
    toggleMark(marks.code)(props.view.state, props.view.dispatch);
    props.view.focus();
  };

  const setHeading = (level: number) => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    if (props.formatState.headingLevel === level) {
      setBlockType(nodes.paragraph)(state, dispatch);
    } else {
      setBlockType(nodes.heading, { level })(state, dispatch);
    }
    props.view.focus();
  };

  // Helper to unwrap a list by replacing it with its items' content
  const unwrapList = () => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    const { from, to, $from } = state.selection;

    // Find the list node wrapping the selection (check ancestors first)
    let listNode: PMNode | null = null;
    let listStart = 0;

    for (let d = $from.depth; d > 0; d--) {
      const node = $from.node(d);
      if (node.type === nodes.bullet_list || node.type === nodes.ordered_list) {
        listNode = node;
        listStart = $from.before(d);
        break;
      }
    }

    // If not found in ancestors, search in selection range (for Ctrl+A)
    if (!listNode) {
      state.doc.nodesBetween(from, to, (node, pos) => {
        if (!listNode && (node.type === nodes.bullet_list || node.type === nodes.ordered_list)) {
          listNode = node;
          listStart = pos;
        }
      });
    }

    if (listNode) {
      const listEnd = listStart + listNode.nodeSize;

      // Check if the original selection was "select all" (AllSelection or spans whole doc)
      const wasSelectAll = state.selection instanceof AllSelection || (from <= 1 && to >= state.doc.content.size - 1);

      // Check if selection is within the list (for selecting all list content after unwrap)
      const selectionWithinList = from >= listStart && to <= listEnd;

      // Collect all content from list items (each list_item contains paragraph(s))
      const fragments: PMNode[] = [];
      listNode.forEach((listItem) => {
        listItem.forEach((child) => {
          fragments.push(child);
        });
      });

      // Calculate the total size of inserted content
      let insertedSize = 0;
      for (const frag of fragments) {
        insertedSize += frag.nodeSize;
      }

      // Replace the list with the collected fragments
      const tr = state.tr.replaceWith(listStart, listEnd, fragments);

      // Restore selection
      if (wasSelectAll) {
        // Original was select-all, so select all in new doc
        tr.setSelection(new AllSelection(tr.doc));
      } else if (from !== to && selectionWithinList) {
        // Selection was within the list - select all the newly inserted content
        try {
          const newFrom = listStart + 1; // Start inside first paragraph
          const newTo = listStart + insertedSize - 1; // End inside last paragraph
          tr.setSelection(TextSelection.create(tr.doc, newFrom, newTo));
        } catch {
          // Fallback to cursor at start
          tr.setSelection(Selection.near(tr.doc.resolve(listStart)));
        }
      } else if (from !== to) {
        // Selection spans outside the list - use mapping
        try {
          const newFrom = tr.mapping.map(from);
          const newTo = tr.mapping.map(to);
          tr.setSelection(TextSelection.create(tr.doc, newFrom, newTo));
        } catch {
          tr.setSelection(Selection.near(tr.doc.resolve(listStart)));
        }
      }
      // If from === to (cursor, no selection), let ProseMirror handle it naturally

      dispatch(tr);
    }
  };

  // Helper to change list type (bullet <-> ordered) by replacing the wrapper node
  const changeListType = (newListType: typeof nodes.bullet_list | typeof nodes.ordered_list) => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    const { from, to, $from } = state.selection;

    // Find the list node wrapping the selection (check ancestors first)
    let listNode: PMNode | null = null;
    let listStart = 0;

    for (let d = $from.depth; d > 0; d--) {
      const node = $from.node(d);
      if (node.type === nodes.bullet_list || node.type === nodes.ordered_list) {
        listNode = node;
        listStart = $from.before(d);
        break;
      }
    }

    // If not found in ancestors, search in selection range (for AllSelection)
    if (!listNode) {
      state.doc.nodesBetween(from, to, (node, pos) => {
        if (!listNode && (node.type === nodes.bullet_list || node.type === nodes.ordered_list)) {
          listNode = node;
          listStart = pos;
        }
      });
    }

    if (listNode) {
      // Check if original selection was "select all"
      const wasSelectAll = state.selection instanceof AllSelection || (from <= 1 && to >= state.doc.content.size - 1);

      // Change the list type while preserving content
      const tr = state.tr.setNodeMarkup(listStart, newListType);

      // Also clear checked attribute from all list items if converting to ordered list
      if (newListType === nodes.ordered_list) {
        tr.doc.nodesBetween(listStart, listStart + listNode.nodeSize, (n, pos) => {
          if (n.type === nodes.list_item && n.attrs.checked !== null) {
            tr.setNodeMarkup(pos, undefined, { checked: null });
          }
        });
      }

      // Restore selection
      if (wasSelectAll) {
        tr.setSelection(new AllSelection(tr.doc));
      } else if (from !== to) {
        try {
          const newFrom = tr.mapping.map(from);
          const newTo = tr.mapping.map(to);
          tr.setSelection(TextSelection.create(tr.doc, newFrom, newTo));
        } catch {
          // Fallback - selection structure changed
        }
      }

      dispatch(tr);
    }
  };

  // Helper to wrap in list while preserving selection
  const wrapInListPreserveSelection = (listType: typeof nodes.bullet_list | typeof nodes.ordered_list) => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    const { from, to } = state.selection;

    // Check if original selection was "select all"
    const wasSelectAll = state.selection instanceof AllSelection || (from <= 1 && to >= state.doc.content.size - 1);

    wrapInList(listType)(state, (tr) => {
      // Restore selection after wrapping
      if (wasSelectAll) {
        tr.setSelection(new AllSelection(tr.doc));
      } else if (from !== to) {
        try {
          const newFrom = tr.mapping.map(from);
          const newTo = tr.mapping.map(to);
          tr.setSelection(TextSelection.create(tr.doc, newFrom, newTo));
        } catch {
          // Fallback - selection structure changed
        }
      }
      dispatch(tr);
    });
  };

  const toggleBulletList = () => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    const { from, to } = state.selection;
    const listState = getCurrentListState();

    if (listState.isBulletList) {
      // Already bullet list - toggle off
      unwrapList();
    } else if (listState.isOrderedList) {
      // Convert ordered to bullet
      changeListType(nodes.bullet_list);
    } else if (listState.isTaskList) {
      // Task list is already a bullet list, just remove checked attrs
      const { $from } = state.selection;
      const wasSelectAll = state.selection instanceof AllSelection || (from <= 1 && to >= state.doc.content.size - 1);

      for (let d = $from.depth; d > 0; d--) {
        const node = $from.node(d);
        if (node.type === nodes.bullet_list) {
          const listStart = $from.before(d);
          let tr = state.tr;
          state.doc.nodesBetween(listStart, listStart + node.nodeSize, (n, pos) => {
            if (n.type === nodes.list_item && n.attrs.checked !== null) {
              tr = tr.setNodeMarkup(pos, undefined, { checked: null });
            }
          });
          if (tr.docChanged) {
            // Restore selection
            if (wasSelectAll) {
              tr.setSelection(new AllSelection(tr.doc));
            } else if (from !== to) {
              try {
                const newFrom = tr.mapping.map(from);
                const newTo = tr.mapping.map(to);
                tr.setSelection(TextSelection.create(tr.doc, newFrom, newTo));
              } catch {
                // Fallback - selection structure changed
              }
            }
            dispatch(tr);
          }
          break;
        }
      }
    } else {
      // Not in any list - wrap in bullet list
      wrapInListPreserveSelection(nodes.bullet_list);
    }
    props.view.focus();
  };

  const toggleOrderedList = () => {
    if (!props.view) return;
    const listState = getCurrentListState();

    if (listState.isOrderedList) {
      // Already ordered list - toggle off
      unwrapList();
    } else if (listState.isBulletList || listState.isTaskList) {
      // Convert bullet/task to ordered
      changeListType(nodes.ordered_list);
    } else {
      // Not in any list - wrap in ordered list
      wrapInListPreserveSelection(nodes.ordered_list);
    }
    props.view.focus();
  };

  const toggleTaskList = () => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    const { from, to, $from } = state.selection;

    // Helper to find list containing position or first list in selection
    const findList = (
      listType: typeof nodes.bullet_list | typeof nodes.ordered_list
    ): { start: number; node: PMNode } | null => {
      // First try to find list as ancestor of selection start
      for (let d = $from.depth; d > 0; d--) {
        const node = $from.node(d);
        if (node.type === listType) {
          return { start: $from.before(d), node };
        }
      }
      // If not found, search in selection range
      let result: { start: number; node: PMNode } | null = null;
      state.doc.nodesBetween(from, to, (node, pos) => {
        if (!result && node.type === listType) {
          result = { start: pos, node };
        }
      });
      return result;
    };

    // Check if original selection was "select all"
    const wasSelectAll = state.selection instanceof AllSelection || (from <= 1 && to >= state.doc.content.size - 1);

    // Helper to restore selection on a transaction
    const restoreSelection = (tr: typeof state.tr) => {
      if (wasSelectAll) {
        tr.setSelection(new AllSelection(tr.doc));
      } else if (from !== to) {
        try {
          const newFrom = tr.mapping.map(from);
          const newTo = tr.mapping.map(to);
          tr.setSelection(TextSelection.create(tr.doc, newFrom, newTo));
        } catch {
          // Fallback - selection structure changed
        }
      }
    };

    const listState = getCurrentListState();

    if (listState.isTaskList) {
      // Toggle off: unwrap task list to paragraphs (consistent with bullet/numbered toggle)
      unwrapList();
    } else if (listState.isBulletList) {
      // Convert bullet list items to task list items
      const found = findList(nodes.bullet_list);
      if (found) {
        let tr = state.tr;
        state.doc.nodesBetween(found.start, found.start + found.node.nodeSize, (n, pos) => {
          if (n.type === nodes.list_item && n.attrs.checked === null) {
            tr = tr.setNodeMarkup(pos, undefined, { checked: false });
          }
        });
        if (tr.docChanged) {
          restoreSelection(tr);
          dispatch(tr);
        }
      }
    } else if (listState.isOrderedList) {
      // Convert ordered list to task list (change type + add checked)
      const found = findList(nodes.ordered_list);
      if (found) {
        let tr = state.tr.setNodeMarkup(found.start, nodes.bullet_list);
        // Need to re-resolve after changing list type
        tr.doc.nodesBetween(found.start, found.start + found.node.nodeSize, (n, pos) => {
          if (n.type === nodes.list_item) {
            tr = tr.setNodeMarkup(pos, undefined, { checked: false });
          }
        });
        restoreSelection(tr);
        dispatch(tr);
      }
    } else {
      // Not in any list - create new task list
      wrapInList(nodes.bullet_list)(state, (tr) => {
        // After wrapping, mark all new list items as task items
        let newTr = tr;
        tr.doc.descendants((node, pos) => {
          if (node.type === nodes.list_item && node.attrs.checked === null) {
            newTr = newTr.setNodeMarkup(pos, undefined, { checked: false });
          }
        });
        restoreSelection(newTr);
        dispatch(newTr);
      });
    }
    props.view.focus();
  };

  const toggleBlockquote = () => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    if (props.formatState.isBlockquote) {
      lift(state, dispatch);
    } else {
      wrapIn(nodes.blockquote)(state, dispatch);
    }
    props.view.focus();
  };

  const toggleCodeBlock = () => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    if (props.formatState.isCodeBlock) {
      setBlockType(nodes.paragraph)(state, dispatch);
    } else {
      setBlockType(nodes.code_block)(state, dispatch);
    }
    props.view.focus();
  };

  return (
    <Show when={props.position}>
      <div
        ref={toolbarRef}
        class={`${styles.floatingToolbar} ${above() ? styles.above : ''}`}
        data-testid="floating-toolbar"
        style={{
          top: `${above() ? props.position?.top : props.position?.bottom}px`,
          left: `${clampedLeft() ?? props.position?.left ?? 0}px`,
        }}
      >
        <div class={styles.toolbarRow}>
          <button class={formatButtonClass(props.formatState.isBold)} onClick={toggleBold} title="Bold (Ctrl+B)">
            B
          </button>
          <button class={formatButtonClass(props.formatState.isItalic)} onClick={toggleItalic} title="Italic (Ctrl+I)">
            I
          </button>
          <button
            class={formatButtonClass(props.formatState.isUnderline)}
            onClick={toggleUnderline}
            title="Underline (Ctrl+U)"
          >
            <u>U</u>
          </button>
          <button
            class={formatButtonClass(props.formatState.isStrikethrough)}
            onClick={toggleStrikethrough}
            title="Strikethrough (Ctrl+Shift+X)"
          >
            <s>S</s>
          </button>
          <button class={formatButtonClass(props.formatState.isCode)} onClick={toggleCode} title="Code (Ctrl+`)">
            {'</>'}
          </button>
        </div>
        <div class={styles.toolbarRow}>
          <button
            class={formatButtonClass(props.formatState.headingLevel === 1)}
            onClick={() => setHeading(1)}
            title="Heading 1"
          >
            H1
          </button>
          <button
            class={formatButtonClass(props.formatState.headingLevel === 2)}
            onClick={() => setHeading(2)}
            title="Heading 2"
          >
            H2
          </button>
          <button
            class={formatButtonClass(props.formatState.headingLevel === 3)}
            onClick={() => setHeading(3)}
            title="Heading 3"
          >
            H3
          </button>
          <span class={styles.separator} />
          <button
            class={formatButtonClass(props.formatState.isBulletList)}
            onClick={toggleBulletList}
            title="Bullet List"
          >
            •
          </button>
          <button
            class={formatButtonClass(props.formatState.isOrderedList)}
            onClick={toggleOrderedList}
            title="Numbered List"
          >
            1.
          </button>
          <button class={formatButtonClass(props.formatState.isTaskList)} onClick={toggleTaskList} title="Task List">
            ☐
          </button>
          <button class={formatButtonClass(props.formatState.isBlockquote)} onClick={toggleBlockquote} title="Quote">
            "
          </button>
          <button class={formatButtonClass(props.formatState.isCodeBlock)} onClick={toggleCodeBlock} title="Code Block">
            {'</>'}
          </button>
        </div>
      </div>
    </Show>
  );
}
