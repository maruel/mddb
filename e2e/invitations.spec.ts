// Exercises manually shared recipient-bound and bearer invitation links without email delivery.

import { randomUUID } from "node:crypto";
import { test, expect, registerUser, createClient, gotoWithSession } from "./helpers";

test("organization bearer links can be copied, claimed once, and revoked", async ({
  page,
  context,
  request,
  browser,
}, testInfo) => {
  const owner = await registerUser(request, "org-link-owner");
  await gotoWithSession(page, owner.token);
  await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });
  const ownerApi = createClient(request, owner.token);
  const account = await ownerApi.getMe();
  const orgId = account.organization_id!;
  await page.goto(`/settings/org/${orgId}`);
  await page.getByRole("button", { name: "Create invite link" }).click();
  const pending = page.getByRole("region", { name: "Pending Invitations" });
  const link = pending.getByRole("textbox", { name: "Invite link" });
  await expect(link).toBeVisible();
  const url = await link.inputValue();
  expect(new URL(url).pathname).toBe("/accept-invitation/org");
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await pending.getByRole("button", { name: "Copy invite link" }).click();
  await expect(pending.getByRole("status")).toHaveText("Invite link copied");
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(url);
  await testInfo.attach("org-invite-link-desktop", {
    body: await page.screenshot({ fullPage: true }),
    contentType: "image/png",
  });

  const recipient = await registerUser(request, "org-link-recipient");
  const recipientApi = createClient(request, recipient.token);
  const recipientContext = await browser.newContext();
  try {
    const recipientPage = await recipientContext.newPage();
    await recipientPage.route(
      "**/api/v1/auth/invitations/org?token=*",
      async (route) => {
        await route.abort("connectionfailed");
      },
      { times: 1 },
    );
    await gotoWithSession(recipientPage, recipient.token, url);
    await expect(recipientPage.getByRole("alert")).toContainText("Try again.");
    await recipientPage.getByRole("button", { name: "Try again", exact: true }).click();
    await recipientPage.getByRole("button", { name: "Join", exact: true }).click();
    await expect(recipientPage).toHaveURL(/\/settings\/user$/);
    const joined = await recipientApi.getMe();
    expect(joined.organizations?.map((org) => org.organization_id)).toEqual([orgId]);
    expect(joined.workspaces ?? []).toEqual([]);
    expect(joined.email_verified).toBeFalsy();
    await recipientPage.goto(url);
    await expect(recipientPage.getByRole("alert")).toContainText("invitation is unavailable");
  } finally {
    await recipientContext.close();
  }

  await page.reload();
  await page.getByRole("button", { name: "Create invite link" }).click();
  await expect(link).toHaveCount(1);
  const revoked = await link.inputValue();
  await pending.getByRole("button", { name: "Revoke invite link" }).click();
  await expect(pending).toHaveCount(0);
  await page.goto(revoked);
  await expect(page.getByRole("alert")).toContainText("invitation is unavailable");
});

test("workspace bearer invitation retains its scope through signup without personal provisioning", async ({
  page,
  request,
  browser,
}, testInfo) => {
  const owner = await registerUser(request, "ws-link-owner");
  await gotoWithSession(page, owner.token);
  await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });
  const ownerApi = createClient(request, owner.token);
  const ownerAccount = await ownerApi.getMe();
  const workspaceId = ownerAccount.workspace_id!;
  const invitation = await ownerApi.ws(workspaceId).createWSInvitation({ email: "", role: "ws:viewer" });
  const context = await browser.newContext({ viewport: { width: 390, height: 844 } });
  try {
    const recipient = await context.newPage();
    const pageErrors: string[] = [];
    recipient.on("pageerror", (error) => pageErrors.push(error.message));
    await recipient.goto(invitation.invite_url);
    await expect(recipient.getByRole("heading", { name: "Invitation", exact: true })).toBeVisible();
    await expect(
      recipient.getByText(`Expires ${new Date(invitation.expires_at * 1000).toLocaleDateString("en-US")}`, {
        exact: true,
      }),
    ).toBeVisible();
    await recipient.getByRole("button", { name: "Register", exact: true }).click();
    await recipient.getByRole("textbox", { name: "Name", exact: true }).fill("Invited account");
    await recipient
      .getByRole("textbox", { name: "Email", exact: true })
      .fill(`invite-signup-${randomUUID()}@example.com`);
    await recipient.getByLabel("Password", { exact: true }).fill("TestPassword123!");
    await recipient.getByRole("button", { name: "Register", exact: true }).click();
    await expect(recipient.getByRole("button", { name: "Join", exact: true })).toBeEnabled();
    expect(new URL(recipient.url()).pathname).toBe("/accept-invitation/ws");
    const recipientApi = createClient(context.request);
    const before = await recipientApi.getMe();
    expect(before.organizations ?? []).toEqual([]);
    expect(before.workspaces ?? []).toEqual([]);
    expect(await recipient.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await testInfo.attach("bearer-invite-accept-mobile", {
      body: await recipient.screenshot({ fullPage: true }),
      contentType: "image/png",
    });
    await recipient.getByRole("button", { name: "Join", exact: true }).click();
    await expect(recipient).toHaveURL(new RegExp(`/w/@${workspaceId}`));
    await expect(recipient.getByRole("main")).toBeVisible();
    const after = await recipientApi.getMe();
    expect(after.organizations?.map((org) => org.organization_id)).toEqual([ownerAccount.organization_id]);
    expect(after.workspaces?.map((workspace) => workspace.workspace_id)).toEqual([workspaceId]);
    expect(after.workspace_role).toBe("ws:viewer");
    expect(after.email_verified).toBeFalsy();
    expect(pageErrors).toEqual([]);
  } finally {
    await context.close();
  }
});

test("recipient-bound invitations require independent email proof and survive OAuth sign-in", async ({
  page,
  request,
  browser,
}) => {
  const owner = await registerUser(request, "bound-link-owner");
  await gotoWithSession(page, owner.token);
  await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });
  const ownerApi = createClient(request, owner.token);
  const ownerAccount = await ownerApi.getMe();
  const orgApi = ownerApi.org(ownerAccount.organization_id!);
  const bound = await orgApi.createOrgInvitation({ email: "test-github@example.com", role: "org:member" });
  const unverified = await registerUser(request, "bound-link-unverified");
  const deniedContext = await browser.newContext();
  const verifiedContext = await browser.newContext();
  try {
    const denied = await deniedContext.newPage();
    await gotoWithSession(denied, unverified.token, bound.invite_url);
    await expect(denied.getByRole("button", { name: "Join", exact: true })).toBeDisabled();
    await expect(denied.getByRole("alert")).toContainText("independently verified");
    const sameAddress = await orgApi.createOrgInvitation({ email: unverified.email, role: "org:member" });
    await denied.goto(sameAddress.invite_url);
    await expect(denied.getByRole("button", { name: "Join", exact: true })).toBeDisabled();
    await orgApi.deleteOrgInvitation(sameAddress.id);
    const deniedAccount = await createClient(request, unverified.token).getMe();
    expect(deniedAccount.organizations ?? []).toEqual([]);

    const verified = await verifiedContext.newPage();
    await verified.goto(bound.invite_url);
    await verified.getByRole("link", { name: "Login with GitHub", exact: true }).click();
    await expect(verified.getByRole("button", { name: "Join", exact: true })).toBeEnabled();
    expect(new URL(verified.url()).pathname).toBe("/accept-invitation/org");
    const verifiedApi = createClient(verifiedContext.request);
    const afterLogin = await verifiedApi.getMe();
    expect(afterLogin.email_verified).toBe(true);
    expect(afterLogin.organizations ?? []).toEqual([]);
    expect(afterLogin.workspaces ?? []).toEqual([]);
    await verified.getByRole("button", { name: "Join", exact: true }).click();
    await expect
      .poll(async () =>
        (await verifiedApi.getMe()).organizations?.some((org) => org.organization_id === ownerAccount.organization_id),
      )
      .toBe(true);

    const workspace = await ownerApi
      .ws(ownerAccount.workspace_id!)
      .createWSInvitation({ email: "test-github@example.com", role: "ws:editor" });
    await verified.goto(workspace.invite_url);
    await verified.getByRole("button", { name: "Join", exact: true }).click();
    await expect(verified).toHaveURL(new RegExp(`/w/@${ownerAccount.workspace_id}`));
    await expect.poll(async () => (await verifiedApi.getMe()).workspace_role).toBe("ws:editor");
    await verified.getByRole("button", { name: "User menu", exact: true }).click();
    await verified.getByRole("menuitem", { name: "Logout", exact: true }).click();
    await expect(verified.getByRole("heading", { name: /login|sign in/i })).toBeVisible();
  } finally {
    await deniedContext.close();
    await verifiedContext.close();
  }
});

test("workspace settings bind invite management to the selected workspace and work for workspace-only admins", async ({
  page,
  request,
  browser,
}) => {
  const owner = await registerUser(request, "selected-workspace-owner");
  await gotoWithSession(page, owner.token);
  await expect(page.locator("aside")).toBeVisible({ timeout: 15000 });
  const ownerApi = createClient(request, owner.token);
  const original = await ownerApi.getMe();
  const selected = await ownerApi
    .org(original.organization_id!)
    .createWorkspace({ name: "Selected invitation workspace" });
  expect((await ownerApi.getMe()).workspace_id).toBe(original.workspace_id);
  await page.goto(`/settings/workspace/${selected.id}`);
  await expect(page.getByRole("button", { name: "Create invite link", exact: true })).toBeEnabled();
  await page.getByRole("button", { name: "Create invite link", exact: true }).click();
  const pending = page.getByRole("region", { name: "Pending Invitations", exact: true });
  const link = pending.getByRole("textbox", { name: "Invite link", exact: true });
  await expect(link).toBeVisible();
  const url = await link.inputValue();
  const token = new URL(url).searchParams.get("token")!;
  expect((await ownerApi.getWSInvitationInfo({ Token: token })).workspace_id).toBe(selected.id);

  const admin = await registerUser(request, "workspace-only-admin");
  const adminApi = createClient(request, admin.token);
  const adminInvitation = await ownerApi.ws(selected.id).createWSInvitation({ email: "", role: "ws:admin" });
  await adminApi.acceptWSInvitation({ token: new URL(adminInvitation.invite_url).searchParams.get("token")! });
  const account = await adminApi.getMe();
  expect(account.organizations?.find((org) => org.organization_id === original.organization_id)?.role).toBe(
    "org:member",
  );
  const adminContext = await browser.newContext();
  try {
    const adminPage = await adminContext.newPage();
    await gotoWithSession(adminPage, admin.token, `/settings/workspace/${selected.id}`);
    await adminPage.reload();
    const ownPending = adminPage.getByRole("region", { name: "Pending Invitations", exact: true });
    await expect(ownPending.getByRole("textbox", { name: "Invite link", exact: true })).toHaveValue(url);
    await adminContext.grantPermissions(["clipboard-read", "clipboard-write"]);
    await ownPending.getByRole("button", { name: "Copy invite link", exact: true }).click();
    await expect(ownPending.getByRole("status")).toHaveText("Invite link copied");
    expect(await adminPage.evaluate(() => navigator.clipboard.readText())).toBe(url);
    await ownPending.getByRole("button", { name: "Revoke invite link", exact: true }).click();
    await expect(ownPending).toHaveCount(0);
    await expect(async () => ownerApi.getWSInvitationInfo({ Token: token })).rejects.toThrow();
  } finally {
    await adminContext.close();
  }
});
