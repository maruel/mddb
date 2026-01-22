import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@solidjs/testing-library';
import type { JSX } from 'solid-js';
import TableGallery from './TableGallery';
import { I18nProvider } from '../i18n';
import type { DataRecordResponse, Property } from '../types';

// Mock CSS module
vi.mock('./TableGallery.module.css', () => ({
  default: {
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
      created: '2024-01-01T00:00:00Z',
      modified: '2024-01-01T00:00:00Z',
    },
    {
      id: 'rec-2',
      data: {
        Title: 'Product B',
        Image: '',
        Description: 'Another product',
        Price: 149,
      },
      created: '2024-01-01T00:00:00Z',
      modified: '2024-01-01T00:00:00Z',
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
      expect(screen.getByText('Product A')).toBeTruthy();
      expect(screen.getByText('Product B')).toBeTruthy();
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
        created: '2024-01-01T00:00:00Z',
        modified: '2024-01-01T00:00:00Z',
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
      const title = screen.getByText('Product A');
      expect(title.tagName.toLowerCase()).toBe('strong');
    });
  });

  it('shows "Untitled" for records without first column value', async () => {
    const recordsWithoutTitle: DataRecordResponse[] = [
      {
        id: 'rec-1',
        data: { Image: 'https://example.com/img.jpg', Description: 'No title' },
        created: '2024-01-01T00:00:00Z',
        modified: '2024-01-01T00:00:00Z',
      },
    ];

    renderWithI18n(() => (
      <TableGallery columns={mockColumnsWithImage} records={recordsWithoutTitle} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      expect(screen.getByText('Untitled')).toBeTruthy();
    });
  });

  it('displays up to 2 additional fields in card body', async () => {
    renderWithI18n(() => (
      <TableGallery columns={mockColumnsWithImage} records={mockRecordsWithImage} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      // Should show Image (col 2) and Description (col 3), but Image is used for display
      // So should show Description and Price (each record has these fields)
      expect(screen.getAllByText('Description:').length).toBeGreaterThan(0);
    });
  });

  it('renders delete button for each card', async () => {
    renderWithI18n(() => (
      <TableGallery columns={mockColumnsWithImage} records={mockRecordsWithImage} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      const deleteButtons = screen.getAllByText('✕');
      expect(deleteButtons.length).toBe(2);
    });
  });

  it('calls onDeleteRecord when delete button is clicked', async () => {
    renderWithI18n(() => (
      <TableGallery columns={mockColumnsWithImage} records={mockRecordsWithImage} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      expect(screen.getByText('Product A')).toBeTruthy();
    });

    const deleteButtons = screen.getAllByText('✕');
    const firstButton = deleteButtons[0];
    if (firstButton) fireEvent.click(firstButton);

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
    const { container } = renderWithI18n(() => (
      <TableGallery columns={mockColumnsWithImage} records={[]} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      const gallery = container.querySelector('.gallery');
      expect(gallery).toBeTruthy();
      expect(gallery?.children.length).toBe(0);
    });
  });

  it('handles table without image column', async () => {
    const recordsNoImage: DataRecordResponse[] = [
      {
        id: 'rec-1',
        data: { Title: 'Article A', Description: 'Some text', Price: 0 },
        created: '2024-01-01T00:00:00Z',
        modified: '2024-01-01T00:00:00Z',
      },
    ];

    renderWithI18n(() => (
      <TableGallery columns={mockColumnsNoImage} records={recordsNoImage} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      // Should still render the card without image section
      expect(screen.getByText('Article A')).toBeTruthy();
    });
  });
});
