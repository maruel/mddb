import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@solidjs/testing-library';
import type { JSX } from 'solid-js';
import TableGrid from './TableGrid';
import { I18nProvider } from '../i18n';
import type { DataRecordResponse, Property } from '@sdk/types.gen';

// Mock CSS module
vi.mock('./TableGrid.module.css', () => ({
  default: {
    container: 'container',
    grid: 'grid',
    card: 'card',
    cardHeader: 'cardHeader',
    cardBody: 'cardBody',
    field: 'field',
    fieldName: 'fieldName',
    titleInput: 'titleInput',
    statusBar: 'statusBar',
    addRecord: 'addRecord',
  },
}));

afterEach(() => {
  cleanup();
});

function renderWithI18n(component: () => JSX.Element) {
  return render(() => <I18nProvider>{component()}</I18nProvider>);
}

describe('TableGrid', () => {
  const mockColumns: Property[] = [
    { name: 'Title', type: 'text' },
    { name: 'Description', type: 'text' },
    { name: 'Price', type: 'number' },
    { name: 'Status', type: 'select' },
  ];

  const mockRecords: DataRecordResponse[] = [
    {
      id: 'rec-1',
      data: { Title: 'Product A', Description: 'A great product', Price: 99, Status: 'active' },
      created: 1704067200,
      modified: 1704067200,
    },
    {
      id: 'rec-2',
      data: { Title: 'Product B', Description: 'Another product', Price: 149, Status: 'draft' },
      created: 1704067200,
      modified: 1704067200,
    },
  ];

  const mockDeleteRecord = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders grid with cards', async () => {
    renderWithI18n(() => <TableGrid columns={mockColumns} records={mockRecords} onDeleteRecord={mockDeleteRecord} />);

    await waitFor(() => {
      expect(screen.getByDisplayValue('Product A')).toBeTruthy();
      expect(screen.getByDisplayValue('Product B')).toBeTruthy();
    });
  });

  it('uses first column value as card title input', async () => {
    renderWithI18n(() => <TableGrid columns={mockColumns} records={mockRecords} onDeleteRecord={mockDeleteRecord} />);

    await waitFor(() => {
      // First column is Title, so "Product A" and "Product B" should be in the title inputs
      const productA = screen.getByDisplayValue('Product A');
      expect(productA).toBeTruthy();
      expect(productA.tagName.toLowerCase()).toBe('input');
      // Title input should NOT be wrapped in a <strong>
      expect(productA.closest('strong')).toBeNull();
    });
  });

  it('shows "Untitled" placeholder for records without first column value', async () => {
    const recordsWithoutTitle: DataRecordResponse[] = [
      {
        id: 'rec-1',
        data: { Description: 'No title', Price: 50 },
        created: 1704067200,
        modified: 1704067200,
      },
    ];

    renderWithI18n(() => (
      <TableGrid columns={mockColumns} records={recordsWithoutTitle} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      expect(screen.getAllByPlaceholderText('Untitled').length).toBeGreaterThan(0);
    });
  });

  it('displays all additional fields in card body (not just first 3)', async () => {
    renderWithI18n(() => <TableGrid columns={mockColumns} records={mockRecords} onDeleteRecord={mockDeleteRecord} />);

    await waitFor(() => {
      // Should show Description, Price, Status (all non-title columns)
      expect(screen.getAllByText('Description').length).toBeGreaterThan(0);
      expect(screen.getAllByText('Price').length).toBeGreaterThan(0);
      expect(screen.getAllByText('Status').length).toBeGreaterThan(0);
    });
  });

  it('shows field values correctly via FieldEditor', async () => {
    renderWithI18n(() => <TableGrid columns={mockColumns} records={mockRecords} onDeleteRecord={mockDeleteRecord} />);

    await waitFor(() => {
      expect(screen.getByDisplayValue('A great product')).toBeTruthy();
      expect(screen.getByDisplayValue('99')).toBeTruthy();
    });
  });

  it('shows empty input for missing field values', async () => {
    const recordsWithMissingFields: DataRecordResponse[] = [
      {
        id: 'rec-1',
        data: { Title: 'Partial', Description: null },
        created: 1704067200,
        modified: 1704067200,
      },
    ];

    renderWithI18n(() => (
      <TableGrid columns={mockColumns} records={recordsWithMissingFields} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      // Description is missing, should be an empty string in at least one input
      expect(screen.getAllByDisplayValue('').length).toBeGreaterThan(0);
    });
  });

  it('shows delete option in context menu', async () => {
    renderWithI18n(() => <TableGrid columns={mockColumns} records={mockRecords} onDeleteRecord={mockDeleteRecord} />);

    await waitFor(() => {
      const cards = document.querySelectorAll('.card');
      expect(cards.length).toBe(2);
    });

    // Trigger context menu on a handle
    const handles = document.querySelectorAll('[aria-label="Drag handle"]');
    if (handles[0]) {
      fireEvent.contextMenu(handles[0]);
    }

    await waitFor(() => {
      expect(screen.getByText(/delete/i)).toBeTruthy();
    });
  });

  it('calls onDeleteRecord when delete option is clicked', async () => {
    renderWithI18n(() => <TableGrid columns={mockColumns} records={mockRecords} onDeleteRecord={mockDeleteRecord} />);

    await waitFor(() => {
      expect(screen.getByDisplayValue('Product A')).toBeTruthy();
    });

    // Trigger context menu
    const handles = document.querySelectorAll('[aria-label="Drag handle"]');
    if (handles[0]) {
      fireEvent.contextMenu(handles[0]);
    }

    await waitFor(() => {
      const deleteOption = screen.getByText(/delete/i);
      fireEvent.click(deleteOption);
    });

    expect(mockDeleteRecord).toHaveBeenCalledWith('rec-1');
  });

  it('shows add record button when onAddRecord is provided', async () => {
    const mockAddRecord = vi.fn();

    renderWithI18n(() => (
      <TableGrid
        columns={mockColumns}
        records={mockRecords}
        onDeleteRecord={mockDeleteRecord}
        onAddRecord={mockAddRecord}
      />
    ));

    await waitFor(() => {
      expect(screen.getByText(/add record/i)).toBeTruthy();
    });

    fireEvent.click(screen.getByText(/add record/i));
    expect(mockAddRecord).toHaveBeenCalledOnce();
  });

  it('hides add record button when onAddRecord is not provided', async () => {
    renderWithI18n(() => <TableGrid columns={mockColumns} records={mockRecords} onDeleteRecord={mockDeleteRecord} />);

    await waitFor(() => {
      expect(screen.queryByText(/add record/i)).toBeNull();
    });
  });

  it('renders empty grid when no records', async () => {
    const { container } = renderWithI18n(() => (
      <TableGrid columns={mockColumns} records={[]} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      const grid = container.querySelector('.grid');
      expect(grid).toBeTruthy();
      expect(grid?.children.length).toBe(0);
    });
  });

  it('handles records with empty columns array', async () => {
    renderWithI18n(() => <TableGrid columns={[]} records={mockRecords} onDeleteRecord={mockDeleteRecord} />);

    // Should render without crashing and show "Untitled" placeholder since no first column
    await waitFor(() => {
      const untitled = screen.getAllByPlaceholderText('Untitled');
      expect(untitled.length).toBe(2);
    });
  });
});
