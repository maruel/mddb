// Tree reconciliation helpers for workspace moves, lazy children, and breadcrumb paths.

import type { NodeResponse } from '@sdk/types.gen';

export interface ReconciledMove {
  movedNode: NodeResponse;
  destinationChildrenLoaded: boolean;
}

export function findNodePath(nodes: NodeResponse[], nodeId: string): NodeResponse[] | null {
  for (const node of nodes) {
    if (node.id === nodeId) return [node];
    if (node.children) {
      const childPath = findNodePath(node.children, nodeId);
      if (childPath) return [node, ...childPath];
    }
  }
  return null;
}

/** Mutates a store draft after a successful server-side move without completing unloaded child lists. */
export function reconcileMovedNode(nodes: NodeResponse[], nodeId: string, newParentId: string): ReconciledMove | null {
  const sourcePath = findNodePath(nodes, nodeId);
  if (!sourcePath) return null;

  const destinationPath = newParentId === '0' ? null : findNodePath(nodes, newParentId);
  if (newParentId !== '0' && (!destinationPath || sourcePath.some((node) => node.id === newParentId))) {
    return null;
  }

  let movedNode: NodeResponse | null = null;
  const removeFromList = (nodeList: NodeResponse[]): boolean => {
    const index = nodeList.findIndex((node) => node.id === nodeId);
    if (index !== -1) {
      const removed = nodeList[index];
      if (!removed) return false;
      nodeList.splice(index, 1);
      movedNode = { ...removed, parent_id: newParentId };
      return true;
    }

    for (const node of nodeList) {
      if (node.children && removeFromList(node.children)) {
        if (node.children.length === 0) node.has_children = false;
        return true;
      }
    }
    return false;
  };

  if (!removeFromList(nodes) || !movedNode) return null;
  const moved = movedNode;

  if (newParentId === '0') {
    nodes.push(moved);
    return { movedNode: moved, destinationChildrenLoaded: true };
  }

  const destination = destinationPath?.[destinationPath.length - 1];
  if (!destination) return null;
  const destinationChildrenLoaded = destination.children !== undefined;
  if (destination.children) destination.children.push(moved);
  destination.has_children = true;
  return { movedNode: moved, destinationChildrenLoaded };
}

/** Returns the selected node's current path, retaining lazy descendants when the destination is not loaded. */
export function reconcileBreadcrumbPath(
  nodes: NodeResponse[],
  currentPath: NodeResponse[],
  selectedNodeId: string | null,
  move: ReconciledMove,
  newParentId: string
): NodeResponse[] {
  if (!selectedNodeId) return currentPath;

  const selectedPath = findNodePath(nodes, selectedNodeId);
  if (selectedPath) return selectedPath;

  const movedPathIndex = currentPath.findIndex((node) => node.id === move.movedNode.id);
  if (movedPathIndex === -1) return currentPath;

  const destinationPath = newParentId === '0' ? [] : findNodePath(nodes, newParentId);
  if (!destinationPath) return currentPath;
  return [...destinationPath, move.movedNode, ...currentPath.slice(movedPathIndex + 1)];
}

export function reconcileSelectedNodeData(
  selectedNodeId: string | null,
  selectedNodeData: NodeResponse | null,
  movedNodeId: string,
  newParentId: string
): NodeResponse | null {
  if (selectedNodeId !== movedNodeId || !selectedNodeData) return selectedNodeData;
  return { ...selectedNodeData, parent_id: newParentId };
}
