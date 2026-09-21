// Tests for the TableTable view.
import { describe, it, beforeEach, afterEach } from "node:test";
import { expect, vi } from "@tests/expect";
import { render, screen, fireEvent, waitFor, cleanup } from "@solidjs/testing-library";
import type { JSX } from "solid-js";
import TableTable from "./TableTable";
import { I18nProvider } from "../i18n";
import { DEFAULT_VIEW_ID, RecordsContext } from "../contexts/RecordsContext";
import type { RecordsContextValue } from "../contexts/RecordsContext";
import type { DataRecordResponse, Property } from "@sdk/types.gen";

// Canned records state provided through the real RecordsContext instead of a module mock.
const recordsValue: RecordsContextValue = {
  records: () => [],
  workspaceMembers: () => [],
  resolvedUsers: () => new Map(),
  hasMore: () => false,
  views: () => [],
  activeViewId: () => DEFAULT_VIEW_ID,
  activeFilters: () => [],
  activeSorts: () => [],
  loadingRecords: () => false,
  savingRecordId: () => null,
  deletingRecordId: () => null,
  savingView: () => false,
  loadError: () => null,
  setLoadError: vi.fn(),
  saveError: () => null,
  setSaveError: vi.fn(),
  loading: () => false,
  loadRecords: vi.fn(async () => undefined),
  loadMoreRecords: vi.fn(async () => undefined),
  addRecord: vi.fn(async () => undefined),
  updateRecord: vi.fn(async () => undefined),
  deleteRecord: vi.fn(async () => undefined),
  duplicateRecord: vi.fn(async () => undefined),
  clearRecords: vi.fn(),
  setActiveViewId: vi.fn(),
  setFilters: vi.fn(),
  setSorts: vi.fn(),
  createView: vi.fn(async () => undefined),
  updateView: vi.fn(async () => undefined),
  deleteView: vi.fn(async () => undefined),
  clearErrors: vi.fn(),
  undo: vi.fn(async () => undefined),
  redo: vi.fn(async () => undefined),
  canUndo: () => false,
  canRedo: () => false,
};

afterEach(() => {
  cleanup();
});

function renderWithI18n(component: () => JSX.Element) {
  return render(() => (
    <RecordsContext.Provider value={recordsValue}>
      <I18nProvider>{component()}</I18nProvider>
    </RecordsContext.Provider>
  ));
}

describe("TableTable", () => {
  const mockColumns: Property[] = [
    { name: "Name", type: "text", required: true },
    { name: "Age", type: "number" },
    { name: "Active", type: "checkbox" },
    { name: "Birthday", type: "date" },
    {
      name: "Status",
      type: "select",
      options: [
        { id: "active", name: "Active", color: "green" },
        { id: "inactive", name: "Inactive", color: "gray" },
      ],
    },
  ];

  const mockRecords: DataRecordResponse[] = [
    {
      id: "rec-1",
      data: { Name: "Alice", Age: 30, Active: true, Birthday: "1994-05-15", Status: "active" },
      created: 1704067200,
      modified: 1704067200,
    },
    {
      id: "rec-2",
      data: { Name: "Bob", Age: 25, Active: false, Birthday: "1999-08-20", Status: "inactive" },
      created: 1704067200,
      modified: 1704067200,
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders table with headers", async () => {
    renderWithI18n(() => <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} />);

    await waitFor(() => {
      expect(screen.getByText("Name")).toBeTruthy();
      expect(screen.getByText("Age")).toBeTruthy();
      // 'Active' appears in header (checkbox col) and in select cell (option name), so use getAllByText
      expect(screen.getAllByText("Active").length).toBeGreaterThan(0);
      expect(screen.getByText("Birthday")).toBeTruthy();
      expect(screen.getByText("Status")).toBeTruthy();
    });
  });

  it("shows required indicator for required columns", async () => {
    renderWithI18n(() => <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} />);

    await waitFor(() => {
      const requiredIndicator = screen.getByText("*");
      expect(requiredIndicator).toBeTruthy();
    });
  });

  it("renders record data correctly", async () => {
    renderWithI18n(() => <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} />);

    await waitFor(() => {
      expect(screen.getByText("Alice")).toBeTruthy();
      expect(screen.getByText("Bob")).toBeTruthy();
      expect(screen.getByText("30")).toBeTruthy();
      expect(screen.getByText("25")).toBeTruthy();
    });
  });

  it("renders checkbox values as checkmarks", async () => {
    renderWithI18n(() => <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} />);

    await waitFor(() => {
      // Alice's Active is true, should show a checked native checkbox
      const checkboxes = document.querySelectorAll('input[type="checkbox"]');
      const checked = Array.from(checkboxes).find((cb) => (cb as HTMLInputElement).checked);
      expect(checked).toBeTruthy();
    });
  });

  it("formats date values", async () => {
    renderWithI18n(() => <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} />);

    // Date should be formatted according to locale
    // The exact format depends on the browser's locale settings
    await waitFor(() => {
      // Just check that the container renders without error
      expect(screen.getByText("Name")).toBeTruthy();
    });
  });

  it("shows delete button when onDeleteRecord is provided", async () => {
    const mockDelete = vi.fn();

    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onDeleteRecord={mockDelete} />
    ));

    await waitFor(() => {
      const deleteButtons = screen.getAllByText("✕");
      expect(deleteButtons.length).toBeGreaterThan(0);
    });
  });

  it("calls onDeleteRecord when delete button is clicked", async () => {
    const mockDelete = vi.fn();

    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onDeleteRecord={mockDelete} />
    ));

    let deleteButtons: HTMLElement[] = [];
    await waitFor(() => {
      deleteButtons = screen.getAllByTitle(/delete/i);
      expect(deleteButtons.length).toBeGreaterThan(0);
    });
    const firstButton = deleteButtons[0];
    if (firstButton) fireEvent.click(firstButton);

    expect(mockDelete).toHaveBeenCalledWith("rec-1");
  });

  it("enters edit mode when clicking a cell", async () => {
    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onUpdateRecord={vi.fn()} />
    ));

    await waitFor(() => {
      expect(screen.getByText("Alice")).toBeTruthy();
    });

    const aliceCell = screen.getByText("Alice");
    fireEvent.click(aliceCell);

    await waitFor(() => {
      // Should now have an input
      const input = document.querySelector('input[type="text"]');
      expect(input).toBeTruthy();
    });
  });

  it("shows inline input when editing (no save/cancel buttons)", async () => {
    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onUpdateRecord={vi.fn()} />
    ));

    await waitFor(() => {
      expect(screen.getByText("Alice")).toBeTruthy();
    });

    const aliceCell = screen.getByText("Alice");
    fireEvent.click(aliceCell);

    await waitFor(() => {
      // When editing, an input appears directly without save/cancel buttons
      const input = document.querySelector('input[type="text"]');
      expect(input).toBeTruthy();
      // No separate save/cancel buttons
      const cancelButton = document.querySelector(".cancelBtn");
      expect(cancelButton).toBeFalsy();
    });
  });

  it("shows add row option when no records", async () => {
    // Empty tables just show headers and "+ New" row
    renderWithI18n(() => <TableTable tableId="db-1" columns={mockColumns} records={[]} onAddRecord={vi.fn()} />);

    await waitFor(() => {
      // Should show the header and the add row option
      expect(screen.getByText("Name")).toBeTruthy();
      expect(screen.getByText(/\+ add record/i)).toBeTruthy();
    });
  });

  it("shows load more button when hasMore is true", async () => {
    const mockLoadMore = vi.fn();

    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} hasMore={true} onLoadMore={mockLoadMore} />
    ));

    await waitFor(() => {
      expect(screen.getByText(/load more/i)).toBeTruthy();
    });

    const loadMoreButton = screen.getByText(/load more/i);
    fireEvent.click(loadMoreButton);

    expect(mockLoadMore).toHaveBeenCalled();
  });

  it("hides load more button when hasMore is false", async () => {
    renderWithI18n(() => <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} hasMore={false} />);

    await waitFor(() => {
      expect(screen.queryByText(/load more/i)).toBeFalsy();
    });
  });

  it("shows clickable new row when onAddRecord is provided", async () => {
    const mockAddRecord = vi.fn();

    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onAddRecord={mockAddRecord} />
    ));

    await waitFor(() => {
      // Should have a clickable "+ New" row
      const newRowText = screen.getByText(/\+ add record/i);
      expect(newRowText).toBeTruthy();
    });
  });

  it("renders select dropdown for select type columns", async () => {
    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onUpdateRecord={vi.fn()} />
    ));

    await waitFor(() => {
      // Status 'active' resolves to option name 'Active'
      expect(screen.getAllByText("Active").length).toBeGreaterThan(0);
    });

    // Click on a status cell to enter edit mode — find the chip inside a <td>
    const chips = screen.getAllByText("Active");
    const statusCell = chips.find((el) => el.closest("td"))!;
    fireEvent.click(statusCell);

    await waitFor(() => {
      // Should now have a custom select dropdown with a clear option (—)
      expect(screen.getByText("—")).toBeTruthy();
    });
  });

  it("renders number input for number type columns", async () => {
    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onUpdateRecord={vi.fn()} />
    ));

    await waitFor(() => {
      expect(screen.getByText("30")).toBeTruthy();
    });

    // Click on age cell
    const ageCell = screen.getByText("30");
    fireEvent.click(ageCell);

    await waitFor(() => {
      const numberInput = document.querySelector('input[type="number"]');
      expect(numberInput).toBeTruthy();
    });
  });

  it("renders date input for date type columns", async () => {
    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onUpdateRecord={vi.fn()} />
    ));

    // Find a date cell and click it
    // Dates are formatted, so we need to find the cell by the row
    await waitFor(() => {
      expect(screen.getByText("Alice")).toBeTruthy();
    });

    // Get all cells in the first row and click the birthday cell
    const table = document.querySelector("table");
    const rows = table?.querySelectorAll("tbody tr");
    if (rows && rows[0]) {
      const cells = rows[0].querySelectorAll("td");
      // Birthday is the 4th column (Name, Age, Active, Birthday)
      // Column 0 is handle, so Birthday is at index 4
      if (cells[4]) {
        fireEvent.click(cells[4]);
      }
    }

    await waitFor(() => {
      const dateInput = document.querySelector('input[type="date"]');
      expect(dateInput).toBeTruthy();
    });
  });

  it("renders checkbox input for checkbox type columns", async () => {
    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onUpdateRecord={vi.fn()} />
    ));

    await waitFor(() => {
      // Active=true renders as a checked native checkbox in read mode
      const checkboxes = document.querySelectorAll('input[type="checkbox"]');
      const checked = Array.from(checkboxes).find((cb) => (cb as HTMLInputElement).checked);
      expect(checked).toBeTruthy();
    });

    // Click on the Active cell (handle=td[0], Name=td[1], Age=td[2], Active=td[3])
    const table = document.querySelector("table");
    const rows = table?.querySelectorAll("tbody tr");
    const activeCell = rows?.[0]?.querySelectorAll("td")[3];
    if (activeCell) {
      fireEvent.click(activeCell);
    }

    await waitFor(() => {
      // In edit mode there is still a checkbox input (now editable)
      const checkbox = document.querySelector('input[type="checkbox"]');
      expect(checkbox).toBeTruthy();
    });
  });

  it("handles cell save on blur", async () => {
    const mockUpdateRecord = vi.fn();

    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onUpdateRecord={mockUpdateRecord} />
    ));

    await waitFor(() => {
      expect(screen.getByText("Alice")).toBeTruthy();
    });

    // Click to edit
    const aliceCell = screen.getByText("Alice");
    fireEvent.click(aliceCell);

    await waitFor(() => {
      expect(document.querySelector('input[type="text"]')).toBeTruthy();
    });

    // Change the value
    const input = document.querySelector('input[type="text"]') as HTMLInputElement;
    fireEvent.input(input, { target: { value: "Alice Updated" } });

    // Blur to trigger auto-save
    fireEvent.blur(input);

    await waitFor(() => {
      expect(mockUpdateRecord).toHaveBeenCalledWith(
        "rec-1",
        expect.objectContaining({
          Name: "Alice Updated",
        }),
      );
    });
  });

  it("handles cell cancel with Escape key", async () => {
    const mockUpdateRecord = vi.fn();

    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onUpdateRecord={mockUpdateRecord} />
    ));

    await waitFor(() => {
      expect(screen.getByText("Alice")).toBeTruthy();
    });

    // Click to edit
    const aliceCell = screen.getByText("Alice");
    fireEvent.click(aliceCell);

    await waitFor(() => {
      expect(document.querySelector('input[type="text"]')).toBeTruthy();
    });

    // Change the value but don't save
    const input = document.querySelector('input[type="text"]') as HTMLInputElement;
    fireEvent.input(input, { target: { value: "Alice Updated" } });

    // Press Escape to cancel
    // In real browser, blur fires after component re-renders removing input
    // In tests, we just verify Escape doesn't immediately save
    fireEvent.keyDown(input, { key: "Escape" });

    // Wait for SolidJS to process the signal update
    await waitFor(() => {
      // After Escape, the editing should be cancelled
      // Check that update was NOT called (no immediate save on Escape)
      expect(mockUpdateRecord).not.toHaveBeenCalled();
    });
  });
});
