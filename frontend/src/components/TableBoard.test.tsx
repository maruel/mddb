import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@solidjs/testing-library';
import type { JSX } from 'solid-js';
import TableBoard from './TableBoard';
import { I18nProvider } from '../i18n';
import type { DataRecordResponse, Property } from '../types.gen';

// Mock CSS module
vi.mock('./TableBoard.module.css', () => ({
  default: {
    board: 'board',
    noGroup: 'noGroup',
    columns: 'columns',
    column: 'column',
    columnHeader: 'columnHeader',
    columnName: 'columnName',
    columnCount: 'columnCount',
    cards: 'cards',
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

describe('TableBoard', () => {
  const mockColumnsWithSelect: Property[] = [
    { name: 'Title', type: 'text' },
    {
      name: 'Status',
      type: 'select',
      options: [
        { id: 'todo', name: 'To Do', color: 'gray' },
        { id: 'in_progress', name: 'In Progress', color: 'blue' },
        { id: 'done', name: 'Done', color: 'green' },
      ],
    },
    { name: 'Priority', type: 'text' },
    { name: 'Assignee', type: 'text' },
  ];

  const mockColumnsWithMultiSelect: Property[] = [
    { name: 'Name', type: 'text' },
    {
      name: 'Tags',
      type: 'multi_select',
      options: [
        { id: 'bug', name: 'Bug' },
        { id: 'feature', name: 'Feature' },
      ],
    },
  ];

  const mockColumnsNoSelect: Property[] = [
    { name: 'Title', type: 'text' },
    { name: 'Description', type: 'text' },
  ];

  const mockRecords: DataRecordResponse[] = [
    {
      id: 'rec-1',
      data: { Title: 'Task 1', Status: 'todo', Priority: 'High', Assignee: 'Alice' },
      created: '2024-01-01T00:00:00Z',
      modified: '2024-01-01T00:00:00Z',
    },
    {
      id: 'rec-2',
      data: { Title: 'Task 2', Status: 'in_progress', Priority: 'Medium', Assignee: 'Bob' },
      created: '2024-01-01T00:00:00Z',
      modified: '2024-01-01T00:00:00Z',
    },
    {
      id: 'rec-3',
      data: { Title: 'Task 3', Status: 'done', Priority: 'Low', Assignee: 'Charlie' },
      created: '2024-01-01T00:00:00Z',
      modified: '2024-01-01T00:00:00Z',
    },
    {
      id: 'rec-4',
      data: { Title: 'Task 4', Status: '', Priority: 'High', Assignee: 'Diana' },
      created: '2024-01-01T00:00:00Z',
      modified: '2024-01-01T00:00:00Z',
    },
  ];

  const mockDeleteRecord = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders board with columns based on select options', async () => {
    renderWithI18n(() => (
      <TableBoard columns={mockColumnsWithSelect} records={mockRecords} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      expect(screen.getByText('To Do')).toBeTruthy();
      expect(screen.getByText('In Progress')).toBeTruthy();
      expect(screen.getByText('Done')).toBeTruthy();
    });
  });

  it('shows "No Group" column for records without status value', async () => {
    renderWithI18n(() => (
      <TableBoard columns={mockColumnsWithSelect} records={mockRecords} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      expect(screen.getByText('No Group')).toBeTruthy();
    });
  });

  it('groups records correctly by status', async () => {
    renderWithI18n(() => (
      <TableBoard columns={mockColumnsWithSelect} records={mockRecords} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      // Task 1 should be in "To Do"
      expect(screen.getByText('Task 1')).toBeTruthy();
      // Task 2 should be in "In Progress"
      expect(screen.getByText('Task 2')).toBeTruthy();
      // Task 3 should be in "Done"
      expect(screen.getByText('Task 3')).toBeTruthy();
      // Task 4 should be in "No Group"
      expect(screen.getByText('Task 4')).toBeTruthy();
    });
  });

  it('shows record count in column headers', async () => {
    renderWithI18n(() => (
      <TableBoard columns={mockColumnsWithSelect} records={mockRecords} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      // Each column should show count
      // To Do: 1, In Progress: 1, Done: 1, No Group: 1
      const counts = screen.getAllByText('1');
      expect(counts.length).toBeGreaterThanOrEqual(4);
    });
  });

  it('uses first column value as card title', async () => {
    renderWithI18n(() => (
      <TableBoard columns={mockColumnsWithSelect} records={mockRecords} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      const task1 = screen.getByText('Task 1');
      expect(task1.tagName.toLowerCase()).toBe('strong');
    });
  });

  it('shows "Untitled" for records without first column value', async () => {
    const recordsWithoutTitle: DataRecordResponse[] = [
      {
        id: 'rec-1',
        data: { Status: 'todo', Priority: 'High' },
        created: '2024-01-01T00:00:00Z',
        modified: '2024-01-01T00:00:00Z',
      },
    ];

    renderWithI18n(() => (
      <TableBoard columns={mockColumnsWithSelect} records={recordsWithoutTitle} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      expect(screen.getByText('Untitled')).toBeTruthy();
    });
  });

  it('displays additional fields in card body (excluding group column)', async () => {
    renderWithI18n(() => (
      <TableBoard columns={mockColumnsWithSelect} records={mockRecords} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      // Should show Priority and Assignee, but not Status (the group column)
      expect(screen.getAllByText('Priority:').length).toBeGreaterThan(0);
      expect(screen.getAllByText('Assignee:').length).toBeGreaterThan(0);
    });
  });

  it('renders delete button for each card', async () => {
    renderWithI18n(() => (
      <TableBoard columns={mockColumnsWithSelect} records={mockRecords} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      const deleteButtons = screen.getAllByText('✕');
      expect(deleteButtons.length).toBe(4);
    });
  });

  it('calls onDeleteRecord when delete button is clicked', async () => {
    renderWithI18n(() => (
      <TableBoard columns={mockColumnsWithSelect} records={mockRecords} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      expect(screen.getByText('Task 1')).toBeTruthy();
    });

    const deleteButtons = screen.getAllByText('✕');
    const firstButton = deleteButtons[0];
    if (firstButton) fireEvent.click(firstButton);

    expect(mockDeleteRecord).toHaveBeenCalled();
  });

  it('shows message when no select column exists', async () => {
    renderWithI18n(() => (
      <TableBoard columns={mockColumnsNoSelect} records={mockRecords} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      expect(screen.getByText(/add a select column/i)).toBeTruthy();
    });
  });

  it('works with multi_select columns', async () => {
    const recordsWithTags: DataRecordResponse[] = [
      {
        id: 'rec-1',
        data: { Name: 'Issue 1', Tags: 'bug' },
        created: '2024-01-01T00:00:00Z',
        modified: '2024-01-01T00:00:00Z',
      },
      {
        id: 'rec-2',
        data: { Name: 'Issue 2', Tags: 'feature' },
        created: '2024-01-01T00:00:00Z',
        modified: '2024-01-01T00:00:00Z',
      },
    ];

    renderWithI18n(() => (
      <TableBoard columns={mockColumnsWithMultiSelect} records={recordsWithTags} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      // Should create columns from options
      expect(screen.getByText('Bug')).toBeTruthy();
      expect(screen.getByText('Feature')).toBeTruthy();
    });
  });

  it('shows empty columns from options even when no records match', async () => {
    const recordsOnlyTodo: DataRecordResponse[] = [
      {
        id: 'rec-1',
        data: { Title: 'Task 1', Status: 'todo', Priority: 'High' },
        created: '2024-01-01T00:00:00Z',
        modified: '2024-01-01T00:00:00Z',
      },
    ];

    renderWithI18n(() => (
      <TableBoard columns={mockColumnsWithSelect} records={recordsOnlyTodo} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      // All columns from options should be visible
      expect(screen.getByText('To Do')).toBeTruthy();
      expect(screen.getByText('In Progress')).toBeTruthy();
      expect(screen.getByText('Done')).toBeTruthy();
    });
  });

  it('handles records with non-option status values', async () => {
    const recordsWithCustomStatus: DataRecordResponse[] = [
      {
        id: 'rec-1',
        data: { Title: 'Task 1', Status: 'custom_status', Priority: 'High' },
        created: '2024-01-01T00:00:00Z',
        modified: '2024-01-01T00:00:00Z',
      },
    ];

    renderWithI18n(() => (
      <TableBoard columns={mockColumnsWithSelect} records={recordsWithCustomStatus} onDeleteRecord={mockDeleteRecord} />
    ));

    await waitFor(() => {
      // Should create a column for the custom status
      expect(screen.getByText('custom_status')).toBeTruthy();
    });
  });

  it('renders empty board when no records', async () => {
    renderWithI18n(() => <TableBoard columns={mockColumnsWithSelect} records={[]} onDeleteRecord={mockDeleteRecord} />);

    await waitFor(() => {
      // Should still show column headers from options
      expect(screen.getByText('To Do')).toBeTruthy();
      expect(screen.getByText('In Progress')).toBeTruthy();
      expect(screen.getByText('Done')).toBeTruthy();
    });
  });
});
