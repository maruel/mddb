// Benchmarks workspace tree reconciliation against a large lazy tree.

import { bench } from "@tests/bench";

import type { NodeResponse } from "@sdk/types.gen";

import { findNodePath, reconcileBreadcrumbPath, reconcileMovedNode } from "./workspaceTree";

function node(id: string, parentId: string, children: NodeResponse[] = []): NodeResponse {
  return {
    id,
    title: `Page ${id}`,
    parent_id: parentId,
    has_page: true,
    has_table: false,
    has_children: children.length > 0,
    children,
    created: 0,
    modified: 0,
  };
}

// Three roots, each holding 40 sections of 25 pages: a workspace that has outgrown
// a sidebar viewport, so lookups cannot lean on small-tree luck.
const pristine: NodeResponse[] = [];
const leafIds: string[] = [];
for (let root = 0; root < 3; root++) {
  const sections: NodeResponse[] = [];
  for (let section = 0; section < 40; section++) {
    const pages: NodeResponse[] = [];
    for (let page = 0; page < 25; page++) {
      const pageNode = node(`p-${root}-${section}-${page}`, `s-${root}-${section}`, []);
      pages.push(pageNode);
      if (root === 2) {
        leafIds.push(pageNode.id);
      }
    }
    sections.push(node(`s-${root}-${section}`, `r-${root}`, pages));
  }
  pristine.push(node(`r-${root}`, "0", sections));
}

// reconcileMovedNode mutates the tree (it splices lists and moves the node), so each
// move iteration consumes a different page and the tree is restored when the cycle wraps.
const tree: NodeResponse[] = structuredClone(pristine);
let moveCursor = 0;

const restoreTree = (): void => {
  tree.length = 0;
  tree.push(...structuredClone(pristine));
  moveCursor = 0;
};

const deepPath = findNodePath(tree, "p-2-39-24");
if (!deepPath || deepPath.length !== 3) {
  throw new Error("expected the deep page to resolve to three path entries");
}
const deepPage = deepPath[deepPath.length - 1];
if (!deepPage) {
  throw new Error("expected the path to end at the page");
}

bench("finds a deep page in a 3000-node tree", () => {
  if (findNodePath(tree, "p-2-39-24") === null) {
    throw new Error("expected a path");
  }
});

bench("reconciles a move within a populated tree", () => {
  const id = leafIds[moveCursor];
  moveCursor += 1;
  if (moveCursor >= leafIds.length || id === undefined) {
    restoreTree();
    return;
  }
  const move = reconcileMovedNode(tree, id, "s-0-0");
  if (!move) {
    throw new Error("expected a move");
  }
});

bench("reconciles a breadcrumb over a deep path", () => {
  const breadcrumb = reconcileBreadcrumbPath(
    tree,
    deepPath,
    deepPage.id,
    { movedNode: deepPage, destinationChildrenLoaded: true },
    "r-0",
  );
  if (breadcrumb.length !== 3) {
    throw new Error("expected three breadcrumb entries");
  }
});
