// Tests recipient link forms, expiry timestamps, and clipboard recovery.

import { afterEach, describe, it } from "node:test";
import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { expect, vi } from "@tests/expect";
import { I18nProvider } from "../../i18n";
import InviteForm from "./InviteForm";

const invitation = {
  id: "invite-1",
  email: "",
  role: "member",
  invite_url: "https://example.test/accept-invitation/org?token=secret",
  expires_at: 1900000000,
};
const clipboard = Object.getOwnPropertyDescriptor(navigator, "clipboard");

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  if (clipboard) Object.defineProperty(navigator, "clipboard", clipboard);
  else Reflect.deleteProperty(navigator, "clipboard");
});

function showForm(
  onInvite: (email: string, role: string) => Promise<boolean>,
  onRevoke: (id: string) => Promise<void>,
) {
  return render(() => (
    <I18nProvider>
      <InviteForm
        roleOptions={[{ value: "member", label: "Member" }]}
        defaultRole="member"
        pendingInvitations={[invitation]}
        onInvite={onInvite}
        onRevoke={onRevoke}
      />
    </I18nProvider>
  ));
}

describe("InviteForm", () => {
  it("formats expiry timestamps from API seconds", async () => {
    showForm(
      async () => true,
      async () => undefined,
    );
    expect(await screen.findByText(/Expires.*2030/)).toBeVisible();
  });

  it("creates a bearer link without an email and retains recipient input after a rejected creation", async () => {
    const invite = vi.fn<(email: string, role: string) => Promise<boolean>>().mockResolvedValue(true);
    showForm(invite, async () => undefined);
    const create = await screen.findByRole("button", { name: "Create invite link" });
    fireEvent.click(create);
    await waitFor(() => expect(invite).toHaveBeenCalledWith("", "member"));
    const recipient = screen.getByLabelText("Recipient email (optional)");
    fireEvent.input(recipient, { target: { value: "recipient@example.test" } });
    invite.mockResolvedValue(false);
    fireEvent.click(create);
    await waitFor(() => expect(invite).toHaveBeenCalledWith("recipient@example.test", "member"));
    expect(recipient).toHaveValue("recipient@example.test");
    invite.mockResolvedValue(true);
    fireEvent.click(create);
    await waitFor(() => expect(recipient).toHaveValue(""));
  });

  it("copies the absolute link and revokes the selected invitation", async () => {
    const writeText = vi.fn<(value: string) => Promise<void>>().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    const revoke = vi.fn<(id: string) => Promise<void>>().mockResolvedValue(undefined);
    showForm(async () => true, revoke);
    fireEvent.click(await screen.findByRole("button", { name: "Copy invite link" }));
    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith("https://example.test/accept-invitation/org?token=secret"),
    );
    await screen.findByRole("status");
    fireEvent.click(screen.getByRole("button", { name: "Revoke invite link" }));
    expect(revoke).toHaveBeenCalledWith("invite-1");
  });

  for (const failure of ["unsupported", "denied"]) {
    it(`keeps a selectable link when clipboard copying is ${failure}`, async () => {
      Object.defineProperty(navigator, "clipboard", {
        configurable: true,
        value:
          failure === "unsupported"
            ? undefined
            : {
                writeText: async () => {
                  throw new Error("Permission denied");
                },
              },
      });
      showForm(
        async () => true,
        async () => undefined,
      );
      fireEvent.click(await screen.findByRole("button", { name: "Copy invite link" }));
      await screen.findByRole("alert");
      const input = screen.getByRole<HTMLInputElement>("textbox", { name: "Invite link" });
      fireEvent.focus(input);
      expect(input.selectionStart).toBe(0);
      expect(input.selectionEnd).toBe(input.value.length);
      expect(input).toHaveValue("https://example.test/accept-invitation/org?token=secret");
    });
  }
});
