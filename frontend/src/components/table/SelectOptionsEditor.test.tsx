// Tests select-option editing, persistence requests, usage counts, and panel dismissal.

import { afterEach, describe, it } from "node:test";
import { expect, vi } from "@tests/expect";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@solidjs/testing-library";
import type { Property } from "@sdk/types.gen";
import { I18nProvider } from "../../i18n";
import { OPTION_COLORS, SelectOptionsEditor } from "./SelectOptionsEditor";

const column: Property = {
  name: "Status",
  type: "select",
  options: [
    { id: "opt1", name: "Alpha" },
    { id: "opt2", name: "Beta" },
  ],
};
const otherColumn: Property = { name: "Title", type: "text" };

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function mount(records: { data: Record<string, unknown> }[] = []) {
  const saved: Property[][] = [];
  const close = vi.fn();
  const updateColumns = (cols: Property[]): Promise<void> => {
    saved.push(cols);
    return Promise.resolve();
  };
  render(() => (
    <I18nProvider>
      <SelectOptionsEditor
        column={column}
        allColumns={[otherColumn, column]}
        records={records}
        position={{ x: 10, y: 10 }}
        onUpdateColumns={updateColumns}
        onClose={close}
      />
    </I18nProvider>
  ));
  const options = () => saved[saved.length - 1]?.find((col) => col.name === "Status")?.options;
  return { saved, options, close };
}

describe("SelectOptionsEditor", () => {
  it("shows the existing options and closes on Escape", () => {
    const { close } = mount();
    expect(screen.getByTestId("option-name-opt1")).toHaveValue("Alpha");
    expect(screen.getByTestId("option-name-opt2")).toHaveValue("Beta");
    fireEvent.keyDown(document, { key: "Escape" });
    expect(close).toHaveBeenCalledOnce();
  });

  it("adds a named option and sends the complete column list", async () => {
    const { saved, options } = mount();
    fireEvent.click(screen.getByTestId("add-option-btn"));
    const inputs = screen.getAllByPlaceholderText("Option name") as HTMLInputElement[];
    const added = inputs[inputs.length - 1];
    if (!added) throw new Error("New option input was not rendered");
    fireEvent.input(added, { target: { value: "Gamma" } });
    fireEvent.blur(added);
    await waitFor(() => expect(saved).toHaveLength(1));
    expect(options()?.map((opt) => opt.name)).toEqual(["Alpha", "Beta", "Gamma"]);
    expect(saved[0]?.[0]).toEqual(otherColumn);
  });

  it("trims a renamed option before saving", async () => {
    const { options } = mount();
    const input = screen.getByTestId("option-name-opt1");
    fireEvent.input(input, { target: { value: "  Renamed  " } });
    fireEvent.blur(input);
    await waitFor(() => expect(options()?.[0]?.name).toBe("Renamed"));
  });

  it("recolors an option and closes its picker", async () => {
    const { options } = mount();
    const color = OPTION_COLORS[0];
    if (!color) throw new Error("Select color palette is empty");
    fireEvent.click(screen.getByTestId("option-color-opt1"));
    expect(screen.getByTestId("swatch-picker")).toBeTruthy();
    fireEvent.click(screen.getByTestId(`swatch-${color}`));
    await waitFor(() => expect(options()?.[0]?.color).toBe(color));
    expect(screen.queryByTestId("swatch-picker")).toBeNull();
  });

  it("deletes an unused option from the saved schema", async () => {
    const { options } = mount();
    fireEvent.click(screen.getByTestId("option-delete-opt2"));
    await waitFor(() => expect(options()?.map((opt) => opt.id)).toEqual(["opt1"]));
    expect(screen.queryByTestId("option-row-opt2")).toBeNull();
  });

  it("shows the number of records using a selected option", () => {
    mount([{ data: { Status: "opt1" } }, { data: { Status: "opt1,opt2" } }]);
    expect(within(screen.getByTestId("option-row-opt1")).getByText("2")).toBeTruthy();
    expect(within(screen.getByTestId("option-row-opt2")).getByText("1")).toBeTruthy();
  });

  it("saves drag order while preserving option IDs", async () => {
    const { options } = mount();
    const first = screen.getByTestId("option-row-opt1");
    const second = screen.getByTestId("option-row-opt2");
    fireEvent.dragStart(second);
    fireEvent.dragOver(first);
    fireEvent.drop(first);
    await waitFor(() => expect(options()?.map((opt) => opt.id)).toEqual(["opt2", "opt1"]));
  });
});
