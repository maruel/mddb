// Tests the model notice chosen for watched MCP resource changes.

import { describe, it } from "node:test";
import { expect } from "@tests/expect";
import { voiceResourceNotice } from "./voiceResources";

describe("voiceResourceNotice", () => {
  it("asks the model to re-read a changed document", () => {
    const notice = voiceResourceNotice({ uri: "mddb://workspaces/w/nodes/n" });
    expect(notice).toContain("changed elsewhere");
    expect(notice).toContain("Re-read");
  });

  it("announces a workspace list change", () => {
    expect(voiceResourceNotice({ listChanged: true })).toContain("documents were added");
  });

  it("stays silent for an empty notification", () => {
    expect(voiceResourceNotice({})).toBeNull();
  });
});
