// Tests for workspace move reconciliation, selected-node metadata, and lazy child lists.

import { describe, expect, it } from 'vitest';
import type { NodeResponse } from '@sdk/types.gen';
import { reconcileBreadcrumbPath, reconcileMovedNode, reconcileSelectedNodeData } from './workspaceTree';

function node(id: string, title: string, partial: Partial<NodeResponse> = {}): NodeResponse {
  return {
    id,
    title,
    parent_id: '0',
    has_page: true,
    has_table: false,
    created: Date.now(),
    modified: Date.now(),
    ...partial,
  };
}

describe('workspace tree move reconciliation', () => {
  it('updates the selected node parent and breadcrumb path after moving it', () => {
    const selected = node('selected', 'Selected page', { parent_id: 'old-parent', content: 'Full page content' });
    const oldParent = node('old-parent', 'Old parent', { has_children: true, children: [selected] });
    const newParent = node('new-parent', 'New parent', { has_children: false, children: [] });
    const tree = [oldParent, newParent];

    const move = reconcileMovedNode(tree, selected.id, newParent.id);
    if (!move) throw new Error('Expected move reconciliation');

    const selectedData = reconcileSelectedNodeData(selected.id, selected, selected.id, newParent.id);
    if (!selectedData) throw new Error('Expected selected node data');
    const breadcrumb = reconcileBreadcrumbPath(tree, [oldParent, selected], selected.id, move, newParent.id).map(
      (item) => (item.id === selected.id ? selectedData : item)
    );

    expect(move.movedNode.parent_id).toBe(newParent.id);
    expect(selectedData?.parent_id).toBe(newParent.id);
    expect(selectedData?.content).toBe('Full page content');
    expect(breadcrumb.map((item) => item.id)).toEqual([newParent.id, selected.id]);
    expect(breadcrumb[1]?.parent_id).toBe(newParent.id);
  });

  it('keeps an unloaded destination lazy while recording that it has children', () => {
    const source = node('source', 'Source');
    const unloadedDestination = node('destination', 'Destination', { has_children: true });
    const tree = [source, unloadedDestination];

    const move = reconcileMovedNode(tree, source.id, unloadedDestination.id);
    if (!move) throw new Error('Expected move reconciliation');

    expect(move.destinationChildrenLoaded).toBe(false);
    expect(unloadedDestination.children).toBeUndefined();
    expect(unloadedDestination.has_children).toBe(true);
    expect(tree.map((item) => item.id)).toEqual([unloadedDestination.id]);
    expect(move.movedNode.parent_id).toBe(unloadedDestination.id);
    expect(
      reconcileBreadcrumbPath(tree, [source], source.id, move, unloadedDestination.id).map((item) => item.id)
    ).toEqual([unloadedDestination.id, source.id]);
  });
});
