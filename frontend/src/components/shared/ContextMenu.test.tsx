// Tests keyboard and pointer interactions of the shared context menu.

import { afterEach, describe, it } from "node:test";
import { expect, vi } from "@tests/expect";
import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { ContextMenu } from "./ContextMenu";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function mount() {
  const action = vi.fn();
  const close = vi.fn();
  render(() => (
    <ContextMenu
      position={{ x: 20, y: 20 }}
      actions={[
        { id: "duplicate", label: "Duplicate" },
        { id: "disabled", label: "Disabled", disabled: true },
        { id: "delete", label: "Delete", danger: true },
      ]}
      onAction={action}
      onClose={close}
    />
  ));
  const duplicate = screen.getByTestId("context-menu-duplicate");
  const disabled = screen.getByTestId("context-menu-disabled");
  const remove = screen.getByTestId("context-menu-delete");
  return { action, close, duplicate, disabled, remove };
}

describe("ContextMenu", () => {
  it("renders accessible actions with the first enabled action focused", () => {
    const { duplicate, disabled, remove } = mount();
    expect(screen.getByRole("menu")).toBeTruthy();
    expect(duplicate).toHaveClass(/focused/);
    expect(disabled).toHaveAttribute("aria-disabled", "true");
    expect(remove).toHaveTextContent("Delete");
  });

  it("moves focus on hover", () => {
    const { duplicate, remove } = mount();
    fireEvent.mouseEnter(remove);
    expect(remove).toHaveClass(/focused/);
    expect(duplicate).not.toHaveClass(/focused/);
  });

  it("navigates enabled actions in both directions and wraps", () => {
    const { duplicate, disabled, remove } = mount();
    fireEvent.keyDown(document, { key: "ArrowDown" });
    expect(remove).toHaveClass(/focused/);
    expect(disabled).not.toHaveClass(/focused/);
    fireEvent.keyDown(document, { key: "ArrowDown" });
    expect(duplicate).toHaveClass(/focused/);
    fireEvent.keyDown(document, { key: "ArrowUp" });
    expect(remove).toHaveClass(/focused/);
  });

  for (const key of ["Enter", " "]) {
    it(`executes the focused action with ${key === " " ? "Space" : key}`, () => {
      const { action, close } = mount();
      fireEvent.keyDown(document, { key: "ArrowDown" });
      fireEvent.keyDown(document, { key });
      expect(action).toHaveBeenCalledWith("delete");
      expect(close).toHaveBeenCalledOnce();
    });
  }

  it("closes on Escape", () => {
    const { close } = mount();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(close).toHaveBeenCalledOnce();
  });

  it("closes when the user clicks outside", async () => {
    const { close } = mount();
    await new Promise((resolve) => setTimeout(resolve, 0));
    fireEvent.click(document.body);
    expect(close).toHaveBeenCalledOnce();
  });
});
