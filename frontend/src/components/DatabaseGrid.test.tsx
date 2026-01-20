import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@solidjs/testing-library';
import DatabaseGrid from './DatabaseGrid';
import { I18nProvider } from '../i18n';
import type { DataRecord, Property } from '../types';

// Mock CSS module
vi.mock('./DatabaseGrid.module.css', () => ({
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

describe('DatabaseGrid', () => {
  const mockColumns: Property[] = [
    { name: 'Title', type: 'text' },
    { name: 'Description', type: 'text' },
    { name: 'Price', type: 'number' },
    { name: 'Status', type: 'select' },
  ];

  const mockRecords: DataRecord[] = [
    {
      id: 'rec-1',
      data: { Title: 'Product A', Description: 'A great product', Price: 99, Status: 'active' },
      created: '2024-01-01T00:00:00Z',
      modified: '2024-01-01T00:00:00Z',
    },
    {
      id: 'rec-2',
      data: { Title: 'Product B', Description: 'Another product', Price: 149, Status: 'draft' },
      created: '2024-01-01T00:00:00Z',
      modified: '2024-01-01T00:00:00Z',
    },
  ];

  const mockDeleteRecord = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders grid with cards', async () => {
    renderWithI18n(() => (
      <DatabaseGrid columns={mockColumns} records={mockRecords} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      expect(screen.getByText('Product A')).toBeTruthy();
      expect(screen.getByText('Product B')).toBeTruthy();
    });
  });

  it('uses first column value as card title', async () => {
    renderWithI18n(() => (
      <DatabaseGrid columns={mockColumns} records={mockRecords} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      // First column is Title, so "Product A" and "Product B" should be headers
      const productA = screen.getByText('Product A');
      expect(productA.tagName.toLowerCase()).toBe('strong');
    });
  });

  it('shows "Untitled" for records without first column value', async () => {
    const recordsWithoutTitle: DataRecord[] = [
      {
        id: 'rec-1',
        data: { Description: 'No title', Price: 50 },
        created: '2024-01-01T00:00:00Z',
        modified: '2024-01-01T00:00:00Z',
      },
    ];

    renderWithI18n(() => (
      <DatabaseGrid
        columns={mockColumns}
        records={recordsWithoutTitle}
        onDeleteRecord={mockDeleteRecord}
      />
    ));

    await waitFor(() => {
      expect(screen.getByText('Untitled')).toBeTruthy();
    });
  });

  it('displays up to 3 additional fields in card body', async () => {
    renderWithI18n(() => (
      <DatabaseGrid columns={mockColumns} records={mockRecords} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      // Should show Description, Price, Status (columns 2-4, indices 1-3)
      // Each record will have these fields, so use getAllByText
      expect(screen.getAllByText('Description:').length).toBeGreaterThan(0);
      expect(screen.getAllByText('Price:').length).toBeGreaterThan(0);
      expect(screen.getAllByText('Status:').length).toBeGreaterThan(0);
    });
  });

  it('shows field values correctly', async () => {
    renderWithI18n(() => (
      <DatabaseGrid columns={mockColumns} records={mockRecords} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      expect(screen.getByText('A great product')).toBeTruthy();
      expect(screen.getByText('99')).toBeTruthy();
      expect(screen.getByText('active')).toBeTruthy();
    });
  });

  it('shows "-" for missing field values', async () => {
    const recordsWithMissingFields: DataRecord[] = [
      {
        id: 'rec-1',
        data: { Title: 'Partial', Description: null },
        created: '2024-01-01T00:00:00Z',
        modified: '2024-01-01T00:00:00Z',
      },
    ];

    renderWithI18n(() => (
      <DatabaseGrid
        columns={mockColumns}
        records={recordsWithMissingFields}
        onDeleteRecord={mockDeleteRecord}
      />
    ));

    await waitFor(() => {
      const dashes = screen.getAllByText('-');
      expect(dashes.length).toBeGreaterThan(0);
    });
  });

  it('renders delete button for each card', async () => {
    renderWithI18n(() => (
      <DatabaseGrid columns={mockColumns} records={mockRecords} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      const deleteButtons = screen.getAllByText('×');
      expect(deleteButtons.length).toBe(2);
    });
  });

  it('calls onDeleteRecord when delete button is clicked', async () => {
    renderWithI18n(() => (
      <DatabaseGrid columns={mockColumns} records={mockRecords} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      expect(screen.getByText('Product A')).toBeTruthy();
    });

    const deleteButtons = screen.getAllByText('×');
    fireEvent.click(deleteButtons[0]);

    expect(mockDeleteRecord).toHaveBeenCalledWith('rec-1');
  });

  it('renders empty grid when no records', async () => {
    const { container } = renderWithI18n(() => (
      <DatabaseGrid columns={mockColumns} records={[]} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      const grid = container.querySelector('.grid');
      expect(grid).toBeTruthy();
      expect(grid?.children.length).toBe(0);
    });
  });

  it('handles records with empty columns array', async () => {
    renderWithI18n(() => (
      <DatabaseGrid columns={[]} records={mockRecords} onDeleteRecord={mockDeleteRecord} />
    ));

    // Should render without crashing and show "Untitled" since no first column
    await waitFor(() => {
      const untitled = screen.getAllByText('Untitled');
      expect(untitled.length).toBe(2);
    });
  });
});
