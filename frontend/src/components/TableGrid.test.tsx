import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@solidjs/testing-library';
import type { JSX } from 'solid-js';
import TableGrid from './TableGrid';
import { I18nProvider } from '../i18n';
import type { DataRecordResponse, Property } from '../types.gen';

// Mock CSS module
vi.mock('./TableGrid.module.css', () => ({
  default: {
    grid: 'grid',
    card: 'card',
    cardHeader: 'cardHeader',
    deleteBtn: 'deleteBtn',
    cardBody: 'cardBody',
    field: 'field',
    fieldName: 'fieldName',
    fieldValue: 'fieldValue',
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

  it('uses first column value as card title', async () => {
    renderWithI18n(() => <TableGrid columns={mockColumns} records={mockRecords} onDeleteRecord={mockDeleteRecord} />);

    await waitFor(() => {
      // First column is Title, so "Product A" and "Product B" should be in inputs inside strong tags
      const productA = screen.getByDisplayValue('Product A');
      expect(productA.closest('strong')).toBeTruthy();
    });
  });

  it('shows "Untitled" for records without first column value', async () => {
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

  it('displays up to 3 additional fields in card body', async () => {
    renderWithI18n(() => <TableGrid columns={mockColumns} records={mockRecords} onDeleteRecord={mockDeleteRecord} />);

    await waitFor(() => {
      // Should show Description, Price, Status (columns 2-4, indices 1-3)
      // Each record will have these fields, so use getAllByText
      expect(screen.getAllByText('Description:').length).toBeGreaterThan(0);
      expect(screen.getAllByText('Price:').length).toBeGreaterThan(0);
      expect(screen.getAllByText('Status:').length).toBeGreaterThan(0);
    });
  });

  it('shows field values correctly', async () => {
    renderWithI18n(() => <TableGrid columns={mockColumns} records={mockRecords} onDeleteRecord={mockDeleteRecord} />);

    await waitFor(() => {
      expect(screen.getByDisplayValue('A great product')).toBeTruthy();
      expect(screen.getByDisplayValue('99')).toBeTruthy();
      expect(screen.getByDisplayValue('active')).toBeTruthy();
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

  it('renders delete button for each card', async () => {
    renderWithI18n(() => <TableGrid columns={mockColumns} records={mockRecords} onDeleteRecord={mockDeleteRecord} />);

    await waitFor(() => {
      const deleteButtons = screen.getAllByText('✕');
      expect(deleteButtons.length).toBe(2);
    });
  });

  it('calls onDeleteRecord when delete button is clicked', async () => {
    renderWithI18n(() => <TableGrid columns={mockColumns} records={mockRecords} onDeleteRecord={mockDeleteRecord} />);

    await waitFor(() => {
      expect(screen.getByDisplayValue('Product A')).toBeTruthy();
    });

    const deleteButtons = screen.getAllByText('✕');
    const firstButton = deleteButtons[0];
    if (firstButton) fireEvent.click(firstButton);

    expect(mockDeleteRecord).toHaveBeenCalledWith('rec-1');
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
