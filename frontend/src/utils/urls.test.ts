// Tests the local invitation return destination accepted by account settings.

import { describe, it } from "node:test";
import { expect } from "@tests/expect";
import { invitationReturnPath } from "./urls";

describe("invitationReturnPath", () => {
  it("keeps only the invitation scope and token", () => {
    expect(invitationReturnPath("/accept-invitation/org?token=org-secret")).toBe(
      "/accept-invitation/org?token=org-secret",
    );
    expect(invitationReturnPath("/accept-invitation/ws?token=workspace%2Bsecret&oauth_error=identity_claimed")).toBe(
      "/accept-invitation/ws?token=workspace%2Bsecret",
    );
  });
  for (const value of [
    null,
    "",
    "https://elsewhere.test/accept-invitation/org?token=secret",
    "//elsewhere.test/accept-invitation/org?token=secret",
    "javascript:alert(1)",
    "/settings/user",
    "/accept-invitation/org",
    "/accept-invitation/org?token=",
    "/accept-invitation/org?token=one&token=two",
    "/accept-invitation/org?token=secret#fragment",
    "/accept-invitation/other?token=secret",
    "/\\elsewhere.test/accept-invitation/org?token=secret",
  ]) {
    it(`rejects unsupported destination ${String(value)}`, () => expect(invitationReturnPath(value)).toBeNull());
  }
});
