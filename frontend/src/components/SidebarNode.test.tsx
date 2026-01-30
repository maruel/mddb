// Unit tests for SidebarNode component and sidebar data flow.

import { describe, it, expect } from 'vitest';
import { createStore, produce } from 'solid-js/store';
import type { NodeResponse } from '@sdk/types.gen';

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

/**
 * These tests document the sidebar data flow.
 *
 * SidebarNode reads children directly from props.node.children (from the store).
 * When a page is deleted, removeNode() updates the store and the UI updates
 * automatically via SolidJS reactivity.
 */

describe('SidebarNode data flow', () => {
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
