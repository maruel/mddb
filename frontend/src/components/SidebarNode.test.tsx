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
 * These tests document the data flow issues with sidebar child node updates.
 *
 * PROBLEM SUMMARY:
 * When creating a subpage, `fetchNodeChildren(parentId)` updates the store's
 * `node.children` array, but SidebarNode doesn't read from `props.node.children`.
 * Instead, it uses:
 * 1. `props.prefetchedChildren` - cache passed from parent component
 * 2. `loadedChildren()` - local signal state set on first fetch
 *
 * This means store updates to `node.children` are NOT reflected in the UI.
 */

describe('SidebarNode data flow analysis', () => {
  describe('fetchNodeChildren store update', () => {
    it('fetchNodeChildren updates node.children in the store', () => {
      // Simulate the nodes store
      const [nodes, setNodes] = createStore<NodeResponse[]>([
        createNode({
          id: 'parent1',
          title: 'Parent Page',
          parent_id: '0',
          has_children: true,
        }),
      ]);

      // Simulate fetchNodeChildren behavior (from WorkspaceContext.tsx:318-335)
      const newChildren: NodeResponse[] = [
        createNode({
          id: 'child1',
          title: 'Child Page',
          parent_id: 'parent1',
          has_children: false,
        }),
      ];

      // This is what fetchNodeChildren does
      setNodes(
        produce((list) => {
          const updateChildren = (nodeList: NodeResponse[]): boolean => {
            for (const node of nodeList) {
              if (node.id === 'parent1') {
                node.children = newChildren;
                return true;
              }
              if (node.children && updateChildren(node.children)) {
                return true;
              }
            }
            return false;
          };
          updateChildren(list);
        })
      );

      // Verify store was updated
      expect(nodes[0]?.children).toBeDefined();
      expect(nodes[0]?.children).toHaveLength(1);
      expect(nodes[0]?.children?.[0]?.id).toBe('child1');
    });

    it('documents that SidebarNode does NOT read props.node.children', () => {
      /**
       * BUG: SidebarNode.tsx children() function (lines 35-45) reads from:
       * 1. props.prefetchedChildren?.[props.node.id] - parent's prefetch cache
       * 2. loadedChildren() - local signal
       *
       * It does NOT read from props.node.children!
       *
       * So when fetchNodeChildren updates node.children in the store,
       * SidebarNode doesn't see the update.
       */

      // SidebarNode's children() function implementation:
      const mockPrefetchedChildren: Record<string, NodeResponse[]> = {};
      let localLoadedChildren: NodeResponse[] | null = null; // Already loaded (empty)

      const children = () => {
        // Check prefetched cache from parent first
        const prefetched = mockPrefetchedChildren['parent1'];
        if (prefetched) return prefetched;

        // Use locally loaded children if available
        if (localLoadedChildren !== null) return localLoadedChildren;

        return [];
      };

      // Scenario: children were already loaded as empty array
      localLoadedChildren = [];

      // Now fetchNodeChildren updates the store, but...
      const storeNodeChildren = [{ id: 'newChild', title: 'New Child' }];

      // SidebarNode still returns empty because it uses local state
      expect(children()).toEqual([]);

      // The store has the new child, but SidebarNode doesn't see it
      expect(storeNodeChildren).toHaveLength(1);
      expect(children()).not.toEqual(storeNodeChildren); // BUG!
    });

    it('documents that loadedChildren is set once and never refreshed', () => {
      /**
       * BUG: SidebarNode.tsx fetchChildren (line 63) has this guard:
       * `if (!props.onFetchChildren || loadedChildren() !== null || isLoadingChildren()) return;`
       *
       * Once loadedChildren is set (even to []), it won't fetch again.
       * There's no mechanism to invalidate or refresh the local cache.
       */

      let loadedChildren: NodeResponse[] | null = null;
      let fetchCount = 0;

      const fetchChildren = async () => {
        // Guard from SidebarNode.tsx:63
        if (loadedChildren !== null) return; // Won't fetch if already loaded!

        fetchCount++;
        loadedChildren = []; // Set to empty on first fetch
      };

      // First fetch works
      fetchChildren();
      expect(fetchCount).toBe(1);
      expect(loadedChildren).toEqual([]);

      // Second fetch is blocked because loadedChildren !== null
      fetchChildren();
      expect(fetchCount).toBe(1); // Still 1, didn't fetch again!

      // Even if we know there are new children, we can't refresh
      // This is the bug - there's no way to invalidate the cache
    });
  });

  describe('expected fix behavior', () => {
    it('SidebarNode should read from props.node.children when available', () => {
      /**
       * FIX: The children() function should also check props.node.children
       * This way, store updates will be reflected in the UI.
       */

      const mockNode: NodeResponse = createNode({
        id: 'parent1',
        title: 'Parent',
        parent_id: '0',
        has_children: true,
        children: [
          createNode({
            id: 'child1',
            title: 'Child from store',
            parent_id: 'parent1',
            has_children: false,
          }),
        ],
      });

      const mockPrefetchedChildren: Record<string, NodeResponse[]> = {};
      const localLoadedChildren: NodeResponse[] | null = null;

      // FIXED children() function - also checks props.node.children
      const childrenFixed = () => {
        // Check prefetched cache from parent first
        const prefetched = mockPrefetchedChildren[mockNode.id];
        if (prefetched) return prefetched;

        // Check store's node.children (NEW!)
        if (mockNode.children && mockNode.children.length > 0) {
          return mockNode.children;
        }

        // Use locally loaded children if available
        if (localLoadedChildren !== null) return localLoadedChildren;

        return [];
      };

      // Now the children from the store are visible
      expect(childrenFixed()).toHaveLength(1);
      expect(childrenFixed()[0]?.id).toBe('child1');
    });
  });
});
