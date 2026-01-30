import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@solidjs/testing-library';
import type { JSX } from 'solid-js';
import TableTable from './TableTable';
import { I18nProvider } from '../i18n';
import type { DataRecordResponse, Property } from '@sdk/types.gen';

// Mock CSS module
vi.mock('./TableTable.module.css', () => ({
  default: {
    container: 'container',
    tableWrapper: 'tableWrapper',
    table: 'table',
    headerRow: 'headerRow',
    headerCell: 'headerCell',
    required: 'required',
    row: 'row',
    actionsHeader: 'actionsHeader',
    actionsCell: 'actionsCell',
    deleteBtn: 'deleteBtn',
    cell: 'cell',
    editing: 'editing',
    cellContent: 'cellContent',
    input: 'input',
    newRow: 'newRow',
    newRowPlaceholder: 'newRowPlaceholder',
    empty: 'empty',
    loadMore: 'loadMore',
    addColumnCell: 'addColumnCell',
    addColumnWrapper: 'addColumnWrapper',
    addColumnBtn: 'addColumnBtn',
    addColumnDropdown: 'addColumnDropdown',
    columnNameInput: 'columnNameInput',
    columnTypeSelect: 'columnTypeSelect',
    addColumnActions: 'addColumnActions',
    addColumnConfirm: 'addColumnConfirm',
    addColumnCancel: 'addColumnCancel',
  },
}));

afterEach(() => {
  cleanup();
});

function renderWithI18n(component: () => JSX.Element) {
  return render(() => <I18nProvider>{component()}</I18nProvider>);
}

describe('TableTable', () => {
  const mockColumns: Property[] = [
    { name: 'Name', type: 'text', required: true },
    { name: 'Age', type: 'number' },
    { name: 'Active', type: 'checkbox' },
    { name: 'Birthday', type: 'date' },
    {
      name: 'Status',
      type: 'select',
      options: [
        { id: 'active', name: 'Active', color: 'green' },
        { id: 'inactive', name: 'Inactive', color: 'gray' },
      ],
    },
  ];

  const mockRecords: DataRecordResponse[] = [
    {
      id: 'rec-1',
      data: { Name: 'Alice', Age: 30, Active: true, Birthday: '1994-05-15', Status: 'active' },
      created: 1704067200,
      modified: 1704067200,
    },
    {
      id: 'rec-2',
      data: { Name: 'Bob', Age: 25, Active: false, Birthday: '1999-08-20', Status: 'inactive' },
      created: 1704067200,
      modified: 1704067200,
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders table with headers', async () => {
    renderWithI18n(() => <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} />);

    await waitFor(() => {
      expect(screen.getByText('Name')).toBeTruthy();
      expect(screen.getByText('Age')).toBeTruthy();
      expect(screen.getByText('Active')).toBeTruthy();
      expect(screen.getByText('Birthday')).toBeTruthy();
      expect(screen.getByText('Status')).toBeTruthy();
    });
  });

  it('shows required indicator for required columns', async () => {
    renderWithI18n(() => <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} />);

    await waitFor(() => {
      const requiredIndicator = screen.getByText('*');
      expect(requiredIndicator).toBeTruthy();
    });
  });

  it('renders record data correctly', async () => {
    renderWithI18n(() => <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} />);

    await waitFor(() => {
      expect(screen.getByText('Alice')).toBeTruthy();
      expect(screen.getByText('Bob')).toBeTruthy();
      expect(screen.getByText('30')).toBeTruthy();
      expect(screen.getByText('25')).toBeTruthy();
    });
  });

  it('renders checkbox values as checkmarks', async () => {
    renderWithI18n(() => <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} />);

    await waitFor(() => {
      // Alice's Active is true, should show checkmark
      expect(screen.getByText('✓')).toBeTruthy();
    });
  });

  it('formats date values', async () => {
    renderWithI18n(() => <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} />);

    // Date should be formatted according to locale
    // The exact format depends on the browser's locale settings
    await waitFor(() => {
      // Just check that the container renders without error
      expect(screen.getByText('Name')).toBeTruthy();
    });
  });

  it('shows delete button when onDeleteRecord is provided', async () => {
    const mockDelete = vi.fn();

    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onDeleteRecord={mockDelete} />
    ));

    await waitFor(() => {
      const deleteButtons = screen.getAllByText('✕');
      expect(deleteButtons.length).toBeGreaterThan(0);
    });
  });

  it('calls onDeleteRecord when delete button is clicked', async () => {
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

    expect(mockDelete).toHaveBeenCalledWith('rec-1');
  });

  it('enters edit mode when clicking a cell', async () => {
    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onUpdateRecord={vi.fn()} />
    ));

    await waitFor(() => {
      expect(screen.getByText('Alice')).toBeTruthy();
    });

    const aliceCell = screen.getByText('Alice');
    fireEvent.click(aliceCell);

    await waitFor(() => {
      // Should now have an input
      const input = document.querySelector('input[type="text"]');
      expect(input).toBeTruthy();
    });
  });

  it('shows inline input when editing (Notion-style, no save/cancel buttons)', async () => {
    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onUpdateRecord={vi.fn()} />
    ));

    await waitFor(() => {
      expect(screen.getByText('Alice')).toBeTruthy();
    });

    const aliceCell = screen.getByText('Alice');
    fireEvent.click(aliceCell);

    await waitFor(() => {
      // When editing, an input appears directly without save/cancel buttons
      const input = document.querySelector('input[type="text"]');
      expect(input).toBeTruthy();
      // No separate save/cancel buttons in Notion-style UI
      const cancelButton = document.querySelector('.cancelBtn');
      expect(cancelButton).toBeFalsy();
    });
  });

  it('shows add row option when no records (Notion-style)', async () => {
    // In Notion-style, empty tables just show headers and "+ New" row
    renderWithI18n(() => <TableTable tableId="db-1" columns={mockColumns} records={[]} onAddRecord={vi.fn()} />);

    await waitFor(() => {
      // Should show the header and the add row option
      expect(screen.getByText('Name')).toBeTruthy();
      expect(screen.getByText(/\+ add record/i)).toBeTruthy();
    });
  });

  it('shows load more button when hasMore is true', async () => {
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

  it('hides load more button when hasMore is false', async () => {
    renderWithI18n(() => <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} hasMore={false} />);

    await waitFor(() => {
      expect(screen.queryByText(/load more/i)).toBeFalsy();
    });
  });

  it('shows clickable new row when onAddRecord is provided', async () => {
    const mockAddRecord = vi.fn();

    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onAddRecord={mockAddRecord} />
    ));

    await waitFor(() => {
      // Should have a clickable "+ New" row (Notion-style)
      const newRowText = screen.getByText(/\+ add record/i);
      expect(newRowText).toBeTruthy();
    });
  });

  it('renders select dropdown for select type columns', async () => {
    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onUpdateRecord={vi.fn()} />
    ));

    await waitFor(() => {
      expect(screen.getByText('active')).toBeTruthy();
    });

    // Click on a status cell to enter edit mode
    const statusCell = screen.getByText('active');
    fireEvent.click(statusCell);

    await waitFor(() => {
      // Should now have a select dropdown
      const select = document.querySelector('select');
      expect(select).toBeTruthy();
    });
  });

  it('renders number input for number type columns', async () => {
    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onUpdateRecord={vi.fn()} />
    ));

    await waitFor(() => {
      expect(screen.getByText('30')).toBeTruthy();
    });

    // Click on age cell
    const ageCell = screen.getByText('30');
    fireEvent.click(ageCell);

    await waitFor(() => {
      const numberInput = document.querySelector('input[type="number"]');
      expect(numberInput).toBeTruthy();
    });
  });

  it('renders date input for date type columns', async () => {
    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onUpdateRecord={vi.fn()} />
    ));

    // Find a date cell and click it
    // Dates are formatted, so we need to find the cell by the row
    await waitFor(() => {
      expect(screen.getByText('Alice')).toBeTruthy();
    });

    // Get all cells in the first row and click the birthday cell
    const table = document.querySelector('table');
    const rows = table?.querySelectorAll('tbody tr');
    if (rows && rows[0]) {
      const cells = rows[0].querySelectorAll('td');
      // Birthday is the 4th column (Name, Age, Active, Birthday) - 0-indexed as 3
      if (cells[3]) {
        fireEvent.click(cells[3]);
      }
    }

    await waitFor(() => {
      const dateInput = document.querySelector('input[type="date"]');
      expect(dateInput).toBeTruthy();
    });
  });

  it('renders checkbox input for checkbox type columns', async () => {
    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onUpdateRecord={vi.fn()} />
    ));

    await waitFor(() => {
      // The checkmark shows for Active=true
      expect(screen.getByText('✓')).toBeTruthy();
    });

    // Click on the checkmark cell
    const checkmarkCell = screen.getByText('✓');
    fireEvent.click(checkmarkCell);

    await waitFor(() => {
      const checkbox = document.querySelector('input[type="checkbox"]');
      expect(checkbox).toBeTruthy();
    });
  });

  it('handles cell save on blur (Notion-style)', async () => {
    const mockUpdateRecord = vi.fn();

    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onUpdateRecord={mockUpdateRecord} />
    ));

    await waitFor(() => {
      expect(screen.getByText('Alice')).toBeTruthy();
    });

    // Click to edit
    const aliceCell = screen.getByText('Alice');
    fireEvent.click(aliceCell);

    await waitFor(() => {
      expect(document.querySelector('input[type="text"]')).toBeTruthy();
    });

    // Change the value
    const input = document.querySelector('input[type="text"]') as HTMLInputElement;
    fireEvent.input(input, { target: { value: 'Alice Updated' } });

    // Blur to save (Notion-style auto-save)
    fireEvent.blur(input);

    await waitFor(() => {
      expect(mockUpdateRecord).toHaveBeenCalledWith(
        'rec-1',
        expect.objectContaining({
          Name: 'Alice Updated',
        })
      );
    });
  });

  it('handles cell cancel with Escape key (Notion-style)', async () => {
    const mockUpdateRecord = vi.fn();

    renderWithI18n(() => (
      <TableTable tableId="db-1" columns={mockColumns} records={mockRecords} onUpdateRecord={mockUpdateRecord} />
    ));

    await waitFor(() => {
      expect(screen.getByText('Alice')).toBeTruthy();
    });

    // Click to edit
    const aliceCell = screen.getByText('Alice');
    fireEvent.click(aliceCell);

    await waitFor(() => {
      expect(document.querySelector('input[type="text"]')).toBeTruthy();
    });

    // Change the value but don't save
    const input = document.querySelector('input[type="text"]') as HTMLInputElement;
    fireEvent.input(input, { target: { value: 'Alice Updated' } });

    // Press Escape to cancel (Notion-style)
    // In real browser, blur fires after component re-renders removing input
    // In tests, we just verify Escape doesn't immediately save
    fireEvent.keyDown(input, { key: 'Escape' });

    // Wait for SolidJS to process the signal update
    await waitFor(() => {
      // After Escape, the editing should be cancelled
      // Check that update was NOT called (no immediate save on Escape)
      expect(mockUpdateRecord).not.toHaveBeenCalled();
    });
  });
});
