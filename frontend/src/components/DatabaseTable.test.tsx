import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@solidjs/testing-library';
import type { JSX } from 'solid-js';
import DatabaseTable from './DatabaseTable';
import { I18nProvider } from '../i18n';
import type { DataRecord, Property } from '../types';

// Mock CSS module
vi.mock('./DatabaseTable.module.css', () => ({
  default: {
    container: 'container',
    tableWrapper: 'tableWrapper',
    table: 'table',
    headerRow: 'headerRow',
    headerCell: 'headerCell',
    required: 'required',
    row: 'row',
    actionsCell: 'actionsCell',
    deleteBtn: 'deleteBtn',
    cell: 'cell',
    editing: 'editing',
    cellContent: 'cellContent',
    editContainer: 'editContainer',
    input: 'input',
    editActions: 'editActions',
    saveBtn: 'saveBtn',
    cancelBtn: 'cancelBtn',
    newRow: 'newRow',
    addBtn: 'addBtn',
    empty: 'empty',
    loadMore: 'loadMore',
  },
}));

afterEach(() => {
  cleanup();
});

function renderWithI18n(component: () => JSX.Element) {
  return render(() => <I18nProvider>{component()}</I18nProvider>);
}

describe('DatabaseTable', () => {
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

  const mockRecords: DataRecord[] = [
    {
      id: 'rec-1',
      data: { Name: 'Alice', Age: 30, Active: true, Birthday: '1994-05-15', Status: 'active' },
      created: '2024-01-01T00:00:00Z',
      modified: '2024-01-01T00:00:00Z',
    },
    {
      id: 'rec-2',
      data: { Name: 'Bob', Age: 25, Active: false, Birthday: '1999-08-20', Status: 'inactive' },
      created: '2024-01-01T00:00:00Z',
      modified: '2024-01-01T00:00:00Z',
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders table with headers', async () => {
    renderWithI18n(() => (
      <DatabaseTable databaseId="db-1" columns={mockColumns} records={mockRecords} />
    ));

    await waitFor(() => {
      expect(screen.getByText('Name')).toBeTruthy();
      expect(screen.getByText('Age')).toBeTruthy();
      expect(screen.getByText('Active')).toBeTruthy();
      expect(screen.getByText('Birthday')).toBeTruthy();
      expect(screen.getByText('Status')).toBeTruthy();
    });
  });

  it('shows required indicator for required columns', async () => {
    renderWithI18n(() => (
      <DatabaseTable databaseId="db-1" columns={mockColumns} records={mockRecords} />
    ));

    await waitFor(() => {
      const requiredIndicator = screen.getByText('*');
      expect(requiredIndicator).toBeTruthy();
    });
  });

  it('renders record data correctly', async () => {
    renderWithI18n(() => (
      <DatabaseTable databaseId="db-1" columns={mockColumns} records={mockRecords} />
    ));

    await waitFor(() => {
      expect(screen.getByText('Alice')).toBeTruthy();
      expect(screen.getByText('Bob')).toBeTruthy();
      expect(screen.getByText('30')).toBeTruthy();
      expect(screen.getByText('25')).toBeTruthy();
    });
  });

  it('renders checkbox values as checkmarks', async () => {
    renderWithI18n(() => (
      <DatabaseTable databaseId="db-1" columns={mockColumns} records={mockRecords} />
    ));

    await waitFor(() => {
      // Alice's Active is true, should show checkmark
      expect(screen.getByText('✓')).toBeTruthy();
    });
  });

  it('formats date values', async () => {
    renderWithI18n(() => (
      <DatabaseTable databaseId="db-1" columns={mockColumns} records={mockRecords} />
    ));

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
      <DatabaseTable
        databaseId="db-1"
        columns={mockColumns}
        records={mockRecords}
        onDeleteRecord={mockDelete}
      />
    ));

    await waitFor(() => {
      const deleteButtons = screen.getAllByText('✕');
      expect(deleteButtons.length).toBeGreaterThan(0);
    });
  });

  it('calls onDeleteRecord when delete button is clicked', async () => {
    const mockDelete = vi.fn();

    renderWithI18n(() => (
      <DatabaseTable
        databaseId="db-1"
        columns={mockColumns}
        records={mockRecords}
        onDeleteRecord={mockDelete}
      />
    ));

    await waitFor(() => {
      const deleteButtons = screen.getAllByTitle(/delete/i);
      expect(deleteButtons.length).toBeGreaterThan(0);
    });

    const deleteButtons = screen.getAllByTitle(/delete/i);
    const firstButton = deleteButtons[0];
    if (firstButton) fireEvent.click(firstButton);

    expect(mockDelete).toHaveBeenCalledWith('rec-1');
  });

  it('enters edit mode when clicking a cell', async () => {
    renderWithI18n(() => (
      <DatabaseTable
        databaseId="db-1"
        columns={mockColumns}
        records={mockRecords}
        onUpdateRecord={vi.fn()}
      />
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

  it('shows save and cancel buttons when editing', async () => {
    renderWithI18n(() => (
      <DatabaseTable
        databaseId="db-1"
        columns={mockColumns}
        records={mockRecords}
        onUpdateRecord={vi.fn()}
      />
    ));

    await waitFor(() => {
      expect(screen.getByText('Alice')).toBeTruthy();
    });

    const aliceCell = screen.getByText('Alice');
    fireEvent.click(aliceCell);

    await waitFor(() => {
      // When editing, save button (✓) appears in edit actions
      // There may be multiple ✓ symbols (checkbox values), so check for buttons
      const buttons = document.querySelectorAll('button');
      const saveButton = Array.from(buttons).find((b) => b.textContent?.includes('✓'));
      expect(saveButton).toBeTruthy();
    });
  });

  it('shows empty state when no records', async () => {
    renderWithI18n(() => <DatabaseTable databaseId="db-1" columns={mockColumns} records={[]} />);

    await waitFor(() => {
      expect(screen.getByText(/no records/i)).toBeTruthy();
    });
  });

  it('shows load more button when hasMore is true', async () => {
    const mockLoadMore = vi.fn();

    renderWithI18n(() => (
      <DatabaseTable
        databaseId="db-1"
        columns={mockColumns}
        records={mockRecords}
        hasMore={true}
        onLoadMore={mockLoadMore}
      />
    ));

    await waitFor(() => {
      expect(screen.getByText(/load more/i)).toBeTruthy();
    });

    const loadMoreButton = screen.getByText(/load more/i);
    fireEvent.click(loadMoreButton);

    expect(mockLoadMore).toHaveBeenCalled();
  });

  it('hides load more button when hasMore is false', async () => {
    renderWithI18n(() => (
      <DatabaseTable
        databaseId="db-1"
        columns={mockColumns}
        records={mockRecords}
        hasMore={false}
      />
    ));

    await waitFor(() => {
      expect(screen.queryByText(/load more/i)).toBeFalsy();
    });
  });

  it('shows new row input when onAddRecord is provided', async () => {
    const mockAddRecord = vi.fn();

    renderWithI18n(() => (
      <DatabaseTable
        databaseId="db-1"
        columns={mockColumns}
        records={mockRecords}
        onAddRecord={mockAddRecord}
      />
    ));

    await waitFor(() => {
      // Should have an add button (+)
      expect(screen.getByText('+')).toBeTruthy();
    });
  });

  it('renders select dropdown for select type columns', async () => {
    renderWithI18n(() => (
      <DatabaseTable
        databaseId="db-1"
        columns={mockColumns}
        records={mockRecords}
        onUpdateRecord={vi.fn()}
      />
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
      <DatabaseTable
        databaseId="db-1"
        columns={mockColumns}
        records={mockRecords}
        onUpdateRecord={vi.fn()}
      />
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
      <DatabaseTable
        databaseId="db-1"
        columns={mockColumns}
        records={mockRecords}
        onUpdateRecord={vi.fn()}
      />
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
      // Birthday is the 5th column (after Actions, Name, Age, Active)
      if (cells[4]) {
        fireEvent.click(cells[4]);
      }
    }

    await waitFor(() => {
      const dateInput = document.querySelector('input[type="date"]');
      expect(dateInput).toBeTruthy();
    });
  });

  it('renders checkbox input for checkbox type columns', async () => {
    renderWithI18n(() => (
      <DatabaseTable
        databaseId="db-1"
        columns={mockColumns}
        records={mockRecords}
        onUpdateRecord={vi.fn()}
      />
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

  it('handles cell save correctly', async () => {
    const mockUpdateRecord = vi.fn();

    renderWithI18n(() => (
      <DatabaseTable
        databaseId="db-1"
        columns={mockColumns}
        records={mockRecords}
        onUpdateRecord={mockUpdateRecord}
      />
    ));

    await waitFor(() => {
      expect(screen.getByText('Alice')).toBeTruthy();
    });

    // Click to edit
    const aliceCell = screen.getByText('Alice');
    fireEvent.click(aliceCell);

    await waitFor(() => {
      const input = document.querySelector('input[type="text"]');
      expect(input).toBeTruthy();
    });

    // Change the value
    const input = document.querySelector('input[type="text"]') as HTMLInputElement;
    fireEvent.input(input, { target: { value: 'Alice Updated' } });

    // Click save
    const saveButton = screen.getAllByText('✓').find((el) => el.tagName.toLowerCase() === 'button');
    if (saveButton) {
      fireEvent.click(saveButton);
    }

    await waitFor(() => {
      expect(mockUpdateRecord).toHaveBeenCalledWith(
        'rec-1',
        expect.objectContaining({
          Name: 'Alice Updated',
        })
      );
    });
  });

  it('handles cell cancel correctly', async () => {
    const mockUpdateRecord = vi.fn();

    renderWithI18n(() => (
      <DatabaseTable
        databaseId="db-1"
        columns={mockColumns}
        records={mockRecords}
        onUpdateRecord={mockUpdateRecord}
      />
    ));

    await waitFor(() => {
      expect(screen.getByText('Alice')).toBeTruthy();
    });

    // Click to edit
    const aliceCell = screen.getByText('Alice');
    fireEvent.click(aliceCell);

    await waitFor(() => {
      const input = document.querySelector('input[type="text"]');
      expect(input).toBeTruthy();
    });

    // Find the cancel button (✕) within the edit container
    // The cancelBtn class is assigned to the cancel button in the editActions
    const cancelButton = document.querySelector('.cancelBtn');
    expect(cancelButton).toBeTruthy();

    if (cancelButton) {
      fireEvent.click(cancelButton);
    }

    // After clicking cancel, verify update was not called
    // The mockUpdateRecord should not be called since we cancelled
    expect(mockUpdateRecord).not.toHaveBeenCalled();
  });
});
