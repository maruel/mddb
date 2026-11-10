// Tests screen-feed routing for records, bounded listing summaries, own moves, removal, and scoped gap recovery.

import { it } from "node:test";
import { expect } from "../../tests/expect";
import type { WorkspaceEvent } from "@sdk/types.gen";
import { workspaceResourceEvents } from "./workspaceResourceEvents";

const event: WorkspaceEvent = {
  type: "records_changed",
  node_id: "table",
  record_id: "row",
  actor_id: "actor",
  modified: 1,
  aspects: ["records"],
};
it("routes record edits to the actual record and query, without invalidating schema or context globally", () => {
  const changes = workspaceResourceEvents("workspace", event);
  expect(changes).toEqual([
    { uri: "mddb://workspaces/workspace/nodes/table/records", aspects: ["records"] },
    { uri: "mddb://workspaces/workspace/nodes/table/records/row", aspects: ["records"] },
  ]);
  expect(workspaceResourceEvents("workspace", { ...event, type: "table_updated", aspects: ["schema"] })).toContainEqual(
    { uri: "mddb://workspaces/workspace/nodes/table/records", aspects: ["schema"] },
  );
});
it("keeps own hierarchy moves distinct from stripped parent child listings", () => {
  const changes = workspaceResourceEvents("workspace", {
    ...event,
    type: "node_moved",
    parent_id: "parent",
    aspects: ["hierarchy"],
  });
  expect(changes).toContainEqual({ uri: "mddb://workspaces/workspace/nodes/table", aspects: ["hierarchy"] });
  expect(changes).toContainEqual({ uri: "mddb://workspaces/workspace/nodes/parent/children", aspects: ["hierarchy"] });
  expect(changes).toContainEqual({ uri: "mddb://workspaces/workspace/nodes/parent", aspects: ["child-presence"] });
});
it("withdraws removed subtree identities and recovers gaps without exposing event provenance", () => {
  const changes = workspaceResourceEvents("workspace", {
    ...event,
    type: "node_deleted",
    affected_ids: ["child"],
    aspects: ["catalog"],
  });
  expect(changes).toContainEqual({ uri: "mddb://workspaces/workspace/nodes/child", aspects: ["removed"] });
  expect(changes).toContainEqual({ uri: "mddb://workspaces/workspace/nodes/child/records", aspects: ["removed"] });
  expect(workspaceResourceEvents("workspace", { ...event, type: "resync" })).toEqual([
    { uri: "mddb://conversation/context" },
    { listChanged: true },
  ]);
  expect(JSON.stringify(changes)).not.toContain("actor");
});

it("routes represented child presence and title separately from unrepresented page bodies", () => {
  const body = workspaceResourceEvents("workspace", {
    ...event,
    type: "node_updated",
    parent_id: "parent",
    aspects: ["body"],
  });
  expect(body.some((change) => change.uri?.endsWith("/children"))).toBe(false);
  const title = workspaceResourceEvents("workspace", {
    ...event,
    type: "node_updated",
    parent_id: "parent",
    aspects: ["summary"],
  });
  expect(title).toContainEqual({ uri: "mddb://workspaces/workspace/nodes/parent/children", aspects: ["summary"] });
  const grandchild = workspaceResourceEvents("workspace", {
    ...event,
    type: "node_created",
    parent_id: "child",
    aspects: ["hierarchy"],
  });
  expect(grandchild).toContainEqual({ uri: "mddb://workspaces/workspace/nodes/child", aspects: ["child-presence"] });
});
