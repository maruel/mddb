// Projects authenticated workspace effects onto resource identities for the existing conversation monitor.

import type { McpResourceNotification } from "@maruel/gomode/web/McpClient";
import type { WorkspaceEvent } from "@sdk/types.gen";

export function workspaceResourceEvents(workspace: string, event: WorkspaceEvent): McpResourceNotification[] {
  if (event.type === "resync") return [{ uri: "mddb://conversation/context" }, { listChanged: true }];
  const notifications: McpResourceNotification[] = [];
  const base = `mddb://workspaces/${workspace}/nodes/`;
  const aspects = event.aspects ?? [];
  if (event.type === "node_deleted") {
    for (const id of new Set([event.node_id, ...(event.affected_ids ?? [])])) {
      notifications.push(
        { uri: base + id, aspects: ["removed"] },
        { uri: base + id + "/records", aspects: ["removed"] },
        { uri: base + id + "/children", aspects: ["removed"] },
      );
    }
  } else {
    if (
      event.type === "node_moved" ||
      aspects.some((aspect) => ["body", "metadata", "schema", "summary"].includes(aspect))
    )
      notifications.push({ uri: base + event.node_id, aspects });
    if (aspects.includes("records")) {
      notifications.push({ uri: base + event.node_id + "/records", aspects: ["records"] });
      if (event.record_id)
        notifications.push({ uri: base + event.node_id + "/records/" + event.record_id, aspects: ["records"] });
    }
    if (aspects.includes("schema")) notifications.push({ uri: base + event.node_id + "/records", aspects: ["schema"] });
  }
  if (aspects.includes("hierarchy")) {
    for (const parent of new Set([
      event.parent_id ?? "0",
      ...(event.type === "node_moved" ? [event.old_parent_id ?? "0"] : []),
    ])) {
      notifications.push({ uri: base + parent + "/children", aspects: ["hierarchy"] });
      if (parent !== "0") notifications.push({ uri: base + parent, aspects: ["child-presence"] });
    }
  } else if (aspects.includes("summary")) {
    notifications.push({ uri: base + (event.parent_id ?? "0") + "/children", aspects: ["summary"] });
  }
  if (aspects.includes("catalog")) notifications.push({ listChanged: true });
  else if (aspects.includes("summary")) notifications.push({ uri: "mddb://catalog" });
  return notifications;
}
