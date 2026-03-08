import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@solidjs/testing-library';
import type { JSX } from 'solid-js';
import TableGallery from './TableGallery';
import { I18nProvider } from '../i18n';
import type { DataRecordResponse, Property } from '@sdk/types.gen';

// Mock CSS module
vi.mock('./TableGallery.module.css', () => ({
  default: {
    container: 'container',
    gallery: 'gallery',
    card: 'card',
    imageContainer: 'imageContainer',
    imagePlaceholder: 'imagePlaceholder',
    image: 'image',
    cardContent: 'cardContent',
    cardHeader: 'cardHeader',
    deleteBtn: 'deleteBtn',
    cardBody: 'cardBody',
    field: 'field',
    fieldName: 'fieldName',
    fieldValue: 'fieldValue',
    empty: 'empty',
    statusBar: 'statusBar',
    titleInput: 'titleInput',
    addRecord: 'addRecord',
  },
}));

afterEach(() => {
  cleanup();
});

function renderWithI18n(component: () => JSX.Element) {
  return render(() => <I18nProvider>{component()}</I18nProvider>);
}

describe('TableGallery', () => {
  const mockColumnsWithImage: Property[] = [
    { name: 'Title', type: 'text' },
    { name: 'Image', type: 'url' },
    { name: 'Description', type: 'text' },
    { name: 'Price', type: 'number' },
  ];

  const mockColumnsWithCover: Property[] = [
    { name: 'Name', type: 'text' },
    { name: 'Cover', type: 'url' },
    { name: 'Category', type: 'text' },
  ];

  const mockColumnsNoImage: Property[] = [
    { name: 'Title', type: 'text' },
    { name: 'Description', type: 'text' },
    { name: 'Price', type: 'number' },
  ];

  const mockRecordsWithImage: DataRecordResponse[] = [
    {
      id: 'rec-1',
      data: {
        Title: 'Product A',
        Image: 'https://example.com/image1.jpg',
        Description: 'Great product',
        Price: 99,
      },
      created: 1704067200,
      modified: 1704067200,
    },
    {
      id: 'rec-2',
      data: {
        Title: 'Product B',
        Image: '',
        Description: 'Another product',
        Price: 149,
      },
      created: 1704067200,
      modified: 1704067200,
    },
  ];

  const mockDeleteRecord = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders gallery with cards', async () => {
    renderWithI18n(() => (
      <TableGallery columns={mockColumnsWithImage} records={mockRecordsWithImage} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      expect(screen.getByDisplayValue('Product A')).toBeTruthy();
      expect(screen.getByDisplayValue('Product B')).toBeTruthy();
    });
  });

  it('detects image column by name containing "image"', async () => {
    renderWithI18n(() => (
      <TableGallery columns={mockColumnsWithImage} records={mockRecordsWithImage} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      const img = document.querySelector('img');
      expect(img).toBeTruthy();
      expect(img?.src).toBe('https://example.com/image1.jpg');
    });
  });

  it('detects image column by name containing "cover"', async () => {
    const recordsWithCover: DataRecordResponse[] = [
      {
        id: 'rec-1',
        data: {
          Name: 'Album A',
          Cover: 'https://example.com/cover.jpg',
          Category: 'Music',
        },
        created: 1704067200,
        modified: 1704067200,
      },
    ];

    renderWithI18n(() => (
      <TableGallery columns={mockColumnsWithCover} records={recordsWithCover} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      const img = document.querySelector('img');
      expect(img).toBeTruthy();
      expect(img?.src).toBe('https://example.com/cover.jpg');
    });
  });

  it('shows placeholder when no image value', async () => {
    renderWithI18n(() => (
      <TableGallery columns={mockColumnsWithImage} records={mockRecordsWithImage} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      // Product B has no image, should show placeholder
      expect(screen.getByText('No Image')).toBeTruthy();
    });
  });

  it('uses first column value as card title', async () => {
    renderWithI18n(() => (
      <TableGallery columns={mockColumnsWithImage} records={mockRecordsWithImage} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      // Title renders as an editable input in the card header
      const title = screen.getByDisplayValue('Product A');
      expect(title.tagName).toBe('INPUT');
    });
  });

  it('shows "Untitled" for records without first column value', async () => {
    const recordsWithoutTitle: DataRecordResponse[] = [
      {
        id: 'rec-1',
        data: { Image: 'https://example.com/img.jpg', Description: 'No title' },
        created: 1704067200,
        modified: 1704067200,
      },
    ];

    renderWithI18n(() => (
      <TableGallery columns={mockColumnsWithImage} records={recordsWithoutTitle} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      expect(screen.getAllByPlaceholderText('Untitled').length).toBeGreaterThan(0);
    });
  });

  it('displays all body fields in card', async () => {
    renderWithI18n(() => (
      <TableGallery columns={mockColumnsWithImage} records={mockRecordsWithImage} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      // Image column is used as cover; remaining body columns (Description, Price) render as field labels.
      // Colon is added via CSS ::after, so DOM text is just the column name.
      expect(screen.getAllByText('Description').length).toBeGreaterThan(0);
      expect(screen.getAllByText('Price').length).toBeGreaterThan(0);
    });
  });

  it('shows delete option in context menu', async () => {
    renderWithI18n(() => (
      <TableGallery columns={mockColumnsWithImage} records={mockRecordsWithImage} onDeleteRecord={mockDeleteRecord} />
    ));

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
    renderWithI18n(() => (
      <TableGallery columns={mockColumnsWithImage} records={mockRecordsWithImage} onDeleteRecord={mockDeleteRecord} />
    ));

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

  it('sets correct alt text on images', async () => {
    renderWithI18n(() => (
      <TableGallery columns={mockColumnsWithImage} records={mockRecordsWithImage} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      const img = document.querySelector('img');
      expect(img?.alt).toBe('Product A');
    });
  });

  it('renders empty gallery when no records', async () => {
    renderWithI18n(() => (
      <TableGallery columns={mockColumnsWithImage} records={[]} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      // Empty state message replaces the gallery grid
      expect(screen.getByText('No records')).toBeTruthy();
    });
  });

  it('shows add record button when onAddRecord is provided', async () => {
    const mockAddRecord = vi.fn();

    renderWithI18n(() => (
      <TableGallery
        columns={mockColumnsWithImage}
        records={mockRecordsWithImage}
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
    renderWithI18n(() => (
      <TableGallery columns={mockColumnsWithImage} records={mockRecordsWithImage} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      expect(screen.queryByText(/add record/i)).toBeNull();
    });
  });

  it('handles table without image column', async () => {
    const recordsNoImage: DataRecordResponse[] = [
      {
        id: 'rec-1',
        data: { Title: 'Article A', Description: 'Some text', Price: 0 },
        created: 1704067200,
        modified: 1704067200,
      },
    ];

    renderWithI18n(() => (
      <TableGallery columns={mockColumnsNoImage} records={recordsNoImage} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      // Should still render the card without image section
      expect(screen.getByDisplayValue('Article A')).toBeTruthy();
    });
  });
});
