// Unit tests for SidebarNode tree navigation, activation, moves, and sidebar data flow.

import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@solidjs/testing-library';
import { createSignal, type JSX } from 'solid-js';
import { createStore, produce } from 'solid-js/store';
import type { NodeResponse } from '@sdk/types.gen';
import { I18nProvider } from '../i18n';
import SidebarNode from './SidebarNode';

afterEach(() => {
  cleanup();
});

// Helper to create a minimal NodeResponse for testing
function createNode(partial: Partial<NodeResponse> & { id: string; title: string }): NodeResponse {
  return {
    parent_id: '0',
    has_children: false,
    has_page: true,
    has_table: false,
    created: Date.now(),
    modified: Date.now(),
    ...partial,
  };
}

interface TreeHarnessProps {
  node: NodeResponse;
  onDeleteNode?: (nodeId: string) => void;
  onMoveNode?: (nodeId: string, newParentId: string) => void;
  onSelect?: (node: NodeResponse) => void;
}

function TreeHarness(props: TreeHarnessProps): JSX.Element {
  const [focusedNodeId, setFocusedNodeId] = createSignal<string | null>(null);
  const [movingNodeId, setMovingNodeId] = createSignal<string | null>(null);

  return (
    <I18nProvider>
      <ul role="tree">
        <SidebarNode
          node={props.node}
          selectedId={null}
          ancestorIds={[]}
          focusedNodeId={focusedNodeId()}
          movingNodeId={movingNodeId()}
          onSelect={props.onSelect ?? vi.fn()}
          onFocusNode={setFocusedNodeId}
          onStartMove={setMovingNodeId}
          onCreateChildPage={vi.fn()}
          onCreateChildTable={vi.fn()}
          onMoveNode={props.onMoveNode}
          onDeleteNode={props.onDeleteNode}
          depth={0}
          isFirstTreeItem={true}
        />
      </ul>
    </I18nProvider>
  );
}

/**
 * These tests document the sidebar data flow.
 *
 * SidebarNode reads children directly from props.node.children (from the store).
 * When a page is deleted, removeNode() updates the store and the UI updates
 * automatically via SolidJS reactivity.
 */

describe('SidebarNode data flow', () => {
  describe('tree interaction', () => {
    const parentNode = () =>
      createNode({
        id: 'parent',
        title: 'Parent',
        has_children: true,
        children: [
          createNode({ id: 'child', title: 'Child', parent_id: 'parent' }),
          createNode({ id: 'target', title: 'Target', parent_id: 'parent' }),
        ],
      });

    it('exposes tree state and moves focus with arrow keys', () => {
      render(() => <TreeHarness node={parentNode()} />);
      const parent = screen.getByRole('treeitem', { name: 'Parent' });
      const child = screen.getByRole('treeitem', { name: 'Child' });

      expect(parent).toHaveAttribute('aria-expanded', 'true');
      expect(parent).toHaveAttribute('aria-level', '1');
      expect(parent).toHaveAttribute('aria-owns', 'tree-group-parent');
      expect(screen.getByRole('group')).toHaveAttribute('id', 'tree-group-parent');
      expect(screen.getAllByRole('treeitem').filter((item) => item.getAttribute('tabindex') === '0')).toHaveLength(1);
      expect(screen.getByTestId('expand-icon-parent')).not.toHaveAttribute('role');
      expect(screen.getByTestId('expand-icon-parent')).toHaveAttribute('aria-hidden', 'true');
      expect(screen.getAllByTestId('delete-node-button')[0]).toHaveAttribute('aria-hidden', 'true');
      parent.focus();
      fireEvent.keyDown(parent, { key: 'ArrowDown' });
      expect(child).toHaveFocus();
      fireEvent.keyDown(child, { key: 'ArrowLeft' });
      expect(parent).toHaveFocus();
    });

    it('activates a focused tree item with Enter', () => {
      const onSelect = vi.fn();
      const node = parentNode();
      render(() => <TreeHarness node={node} onSelect={onSelect} />);

      const parent = screen.getByRole('treeitem', { name: 'Parent' });
      fireEvent.keyDown(parent, { key: 'Enter' });

      expect(onSelect).toHaveBeenCalledWith(node);
    });

    it('moves roving focus to a parent before collapsing a focused descendant', () => {
      render(() => <TreeHarness node={parentNode()} />);
      const parent = screen.getByRole('treeitem', { name: 'Parent' });
      const child = screen.getByRole('treeitem', { name: 'Child' });

      child.focus();
      fireEvent.click(screen.getByTestId('expand-icon-parent'));

      expect(parent).toHaveFocus();
      expect(screen.queryByRole('treeitem', { name: 'Child' })).toBeNull();
      expect(parent).toHaveAttribute('tabindex', '0');
    });

    it('moves roving focus before deleting a focused non-selected node', async () => {
      const onDeleteNode = vi.fn();
      render(() => <TreeHarness node={parentNode()} onDeleteNode={onDeleteNode} />);
      const parent = screen.getByRole('treeitem', { name: 'Parent' });
      const child = screen.getByRole('treeitem', { name: 'Child' });
      const childWrapper = screen.getByTestId('sidebar-node-child');
      const deleteButton = childWrapper.querySelector<HTMLButtonElement>('[data-testid="delete-node-button"]');
      if (!deleteButton) throw new Error('Expected a delete button');

      child.focus();
      fireEvent.click(deleteButton);

      await waitFor(() => expect(parent).toHaveFocus());
      expect(onDeleteNode).toHaveBeenCalledWith('child');
    });

    it('restores focus after keyboard context-menu actions and moves to a focused target', async () => {
      const onMoveNode = vi.fn();
      render(() => <TreeHarness node={parentNode()} onMoveNode={onMoveNode} />);
      const parent = screen.getByRole('treeitem', { name: 'Parent' });
      const source = screen.getByRole('treeitem', { name: 'Child' });
      const target = screen.getByRole('treeitem', { name: 'Target' });

      parent.focus();
      fireEvent.keyDown(parent, { key: 'ArrowDown' });
      expect(source).toHaveFocus();
      fireEvent.keyDown(source, { key: 'F10', shiftKey: true });
      await waitFor(() => expect(screen.getByTestId('context-menu-move-node')).toBeTruthy());
      fireEvent.click(screen.getByTestId('context-menu-move-node'));

      await waitFor(() => expect(source).toHaveFocus());
      fireEvent.keyDown(source, { key: 'ArrowDown' });
      expect(target).toHaveFocus();
      fireEvent.keyDown(target, { key: 'v', ctrlKey: true });

      expect(onMoveNode).toHaveBeenCalledWith('child', 'target');
    });
  });

  describe('store-based children', () => {
    it('children come from node.children in the store', () => {
      const [nodes] = createStore<NodeResponse[]>([
        createNode({
          id: 'parent1',
          title: 'Parent Page',
          parent_id: '0',
          has_children: true,
          children: [
            createNode({
              id: 'child1',
              title: 'Child Page',
              parent_id: 'parent1',
            }),
          ],
        }),
      ]);

      // Children are directly available from the store
      // eslint-disable-next-line solid/reactivity -- test assertion: reading store values synchronously
      expect(nodes[0]?.children).toHaveLength(1);
      // eslint-disable-next-line solid/reactivity
      expect(nodes[0]?.children?.[0]?.id).toBe('child1');
    });

    it('fetchNodeChildren updates node.children in the store', () => {
      const [nodes, setNodes] = createStore<NodeResponse[]>([
        createNode({
          id: 'parent1',
          title: 'Parent Page',
          parent_id: '0',
          has_children: true,
        }),
      ]);

      // Initially no children loaded
      // eslint-disable-next-line solid/reactivity
      expect(nodes[0]?.children).toBeUndefined();

      // Simulate fetchNodeChildren updating the store
      const newChildren: NodeResponse[] = [
        createNode({
          id: 'child1',
          title: 'Child Page',
          parent_id: 'parent1',
        }),
      ];

      setNodes(
        produce((list) => {
          const node = list[0];
          if (node) {
            node.children = newChildren;
            node.has_children = true;
          }
        })
      );

      // Children are now in the store
      // eslint-disable-next-line solid/reactivity
      expect(nodes[0]?.children).toHaveLength(1);
      // eslint-disable-next-line solid/reactivity
      expect(nodes[0]?.children?.[0]?.id).toBe('child1');
    });
  });

  describe('removeNode function', () => {
    it('removes a root-level node from the store', () => {
      const [nodes, setNodes] = createStore<NodeResponse[]>([
        createNode({ id: 'page1', title: 'Page 1', parent_id: '0' }),
        createNode({ id: 'page2', title: 'Page 2', parent_id: '0' }),
      ]);

      expect(nodes).toHaveLength(2);

      // Simulate removeNode
      setNodes(
        produce((list) => {
          const index = list.findIndex((n) => n.id === 'page1');
          if (index !== -1) list.splice(index, 1);
        })
      );

      expect(nodes).toHaveLength(1);
      // eslint-disable-next-line solid/reactivity
      expect(nodes[0]?.id).toBe('page2');
    });

    it('removes a child node from its parent', () => {
      const [nodes, setNodes] = createStore<NodeResponse[]>([
        createNode({
          id: 'parent1',
          title: 'Parent',
          parent_id: '0',
          has_children: true,
          children: [
            createNode({ id: 'child1', title: 'Child 1', parent_id: 'parent1' }),
            createNode({ id: 'child2', title: 'Child 2', parent_id: 'parent1' }),
          ],
        }),
      ]);

      // eslint-disable-next-line solid/reactivity
      expect(nodes[0]?.children).toHaveLength(2);

      // Simulate removeNode for child1
      setNodes(
        produce((list) => {
          const removeFromList = (nodeList: NodeResponse[]): boolean => {
            const index = nodeList.findIndex((n) => n.id === 'child1');
            if (index !== -1) {
              nodeList.splice(index, 1);
              return true;
            }
            for (const node of nodeList) {
              if (node.children && removeFromList(node.children)) {
                if (node.children.length === 0) {
                  node.has_children = false;
                }
                return true;
              }
            }
            return false;
          };
          removeFromList(list);
        })
      );

      // eslint-disable-next-line solid/reactivity
      expect(nodes[0]?.children).toHaveLength(1);
      // eslint-disable-next-line solid/reactivity
      expect(nodes[0]?.children?.[0]?.id).toBe('child2');
    });

    it('removes a grandchild node and updates has_children', () => {
      const [nodes, setNodes] = createStore<NodeResponse[]>([
        createNode({
          id: 'root',
          title: 'Root',
          parent_id: '0',
          has_children: true,
          children: [
            createNode({
              id: 'child',
              title: 'Child',
              parent_id: 'root',
              has_children: true,
              children: [createNode({ id: 'grandchild', title: 'Grandchild', parent_id: 'child' })],
            }),
          ],
        }),
      ]);

      // eslint-disable-next-line solid/reactivity
      const child = nodes[0]?.children?.[0];
      expect(child?.children).toHaveLength(1);
      expect(child?.has_children).toBe(true);

      // Simulate removeNode for grandchild
      setNodes(
        produce((list) => {
          const removeFromList = (nodeList: NodeResponse[]): boolean => {
            const index = nodeList.findIndex((n) => n.id === 'grandchild');
            if (index !== -1) {
              nodeList.splice(index, 1);
              return true;
            }
            for (const node of nodeList) {
              if (node.children && removeFromList(node.children)) {
                if (node.children.length === 0) {
                  node.has_children = false;
                }
                return true;
              }
            }
            return false;
          };
          removeFromList(list);
        })
      );

      // eslint-disable-next-line solid/reactivity
      const childAfter = nodes[0]?.children?.[0];
      expect(childAfter?.children).toHaveLength(0);
      expect(childAfter?.has_children).toBe(false);
    });
  });
});
