// Tests acknowledged workspace creation and import retry without repeating successful mutations.

import { afterEach, it } from "node:test";
import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { expect, vi } from "@tests/expect";
import { I18nProvider } from "../i18n";
import CreateWorkspaceModal from "./CreateWorkspaceModal";
import NotionImportModal from "./NotionImportModal";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

for (const kind of ["workspace", "import"] as const) {
  it(`retries opening an acknowledged ${kind} without repeating its mutation`, async () => {
    const created = vi.fn(async () => "created-workspace");
    const open = vi
      .fn<(_id: string) => Promise<boolean>>()
      .mockRejectedValueOnce(new Error("Opening denied"))
      .mockResolvedValueOnce(true);
    const close = vi.fn();
    render(() => (
      <I18nProvider initialLocale="en">
        {kind === "workspace" ? (
          <CreateWorkspaceModal onCreate={created} onOpen={open} onClose={close} />
        ) : (
          <NotionImportModal onImport={created} onOpen={open} onClose={close} />
        )}
      </I18nProvider>
    ));
    const input = await screen.findByLabelText(kind === "workspace" ? "Workspace Name" : "Internal Integration Secret");
    fireEvent.input(input, { target: { value: kind === "workspace" ? "New workspace" : "private-token" } });
    fireEvent.click(screen.getByRole("button", { name: kind === "workspace" ? "Create" : "Start Import" }));
    await screen.findByText("Error: Opening denied");
    expect(close).not.toHaveBeenCalled();
    expect(input).toBeDisabled();
    expect(created).toHaveBeenCalledOnce();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    await waitFor(() => expect(close).toHaveBeenCalledOnce());
    expect(created).toHaveBeenCalledOnce();
    expect(open).toHaveBeenNthCalledWith(1, "created-workspace");
    expect(open).toHaveBeenNthCalledWith(2, "created-workspace");
  });
}

it("does not close or show an error for a cancelled workspace activation", async () => {
  const close = vi.fn();
  render(() => (
    <I18nProvider initialLocale="en">
      <CreateWorkspaceModal
        onCreate={() => Promise.resolve("created")}
        onOpen={() => Promise.resolve(false)}
        onClose={close}
      />
    </I18nProvider>
  ));
  fireEvent.input(await screen.findByLabelText("Workspace Name"), { target: { value: "New workspace" } });
  fireEvent.click(screen.getByRole("button", { name: "Create" }));
  await screen.findByRole("button", { name: "Try again" });
  expect(close).not.toHaveBeenCalled();
  expect(screen.queryByText(/Error:/)).not.toBeInTheDocument();
});

it("does not activate an acknowledged creation after its dialog is dismissed", async () => {
  const pending = Promise.withResolvers<string | null>();
  const open = vi.fn(async () => true);
  render(() => (
    <I18nProvider initialLocale="en">
      <CreateWorkspaceModal onCreate={() => pending.promise} onOpen={open} onClose={() => undefined} />
    </I18nProvider>
  ));
  fireEvent.input(await screen.findByLabelText("Workspace Name"), { target: { value: "New workspace" } });
  fireEvent.click(screen.getByRole("button", { name: "Create" }));
  cleanup();
  pending.resolve("created");
  await pending.promise;
  expect(open).not.toHaveBeenCalled();
});
