import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@solidjs/testing-library';
import type { JSX } from 'solid-js';
import App from './App';
import { I18nProvider } from './i18n';
import type { UserResponse, NodeResponse, DataRecordResponse } from './types.gen';
import { UserRoleViewer, UserRoleAdmin } from './types.gen';

// Mock CSS modules
vi.mock('./App.module.css', () => ({
  default: {
    app: 'app',
    header: 'header',
    headerTitle: 'headerTitle',
    userInfo: 'userInfo',
    orgSwitcher: 'orgSwitcher',
    orgName: 'orgName',
    createOrgButton: 'createOrgButton',
    logoutButton: 'logoutButton',
    container: 'container',
    sidebar: 'sidebar',
    sidebarHeader: 'sidebarHeader',
    sidebarActions: 'sidebarActions',
    sidebarFooter: 'sidebarFooter',
    loading: 'loading',
    pageList: 'pageList',
    main: 'main',
    breadcrumbs: 'breadcrumbs',
    breadcrumbSeparator: 'breadcrumbSeparator',
    breadcrumbItem: 'breadcrumbItem',
    error: 'error',
    welcome: 'welcome',
    createForm: 'createForm',
    titleInput: 'titleInput',
    welcomeActions: 'welcomeActions',
    createButton: 'createButton',
    editor: 'editor',
    editorHeader: 'editorHeader',
    editorStatus: 'editorStatus',
    unsavedIndicator: 'unsavedIndicator',
    savingIndicator: 'savingIndicator',
    savedIndicator: 'savedIndicator',
    editorActions: 'editorActions',
    historyPanel: 'historyPanel',
    historyList: 'historyList',
    historyItem: 'historyItem',
    historyMeta: 'historyMeta',
    historyDate: 'historyDate',
    historyHash: 'historyHash',
    historyMessage: 'historyMessage',
    nodeContent: 'nodeContent',
    editorContent: 'editorContent',
    contentInput: 'contentInput',
    tableView: 'tableView',
    tableHeader: 'tableHeader',
    viewToggle: 'viewToggle',
    active: 'active',
  },
}));

// Mock child components that have their own tests
vi.mock('./components/SidebarNode', () => ({
  default: (props: { node: NodeResponse; selectedId: string | null; onSelect: (n: NodeResponse) => void }) => (
    <li data-testid={`sidebar-node-${props.node.id}`} onClick={() => props.onSelect(props.node)}>
      {props.node.title}
    </li>
  ),
}));

vi.mock('./components/MarkdownPreview', () => ({
  default: (props: { content: string }) => <div data-testid="markdown-preview">{props.content}</div>,
}));

vi.mock('./components/TableTable', () => ({
  default: () => <div data-testid="table-table">TableTable</div>,
}));

vi.mock('./components/TableGrid', () => ({
  default: () => <div data-testid="table-grid">TableGrid</div>,
}));

vi.mock('./components/TableGallery', () => ({
  default: () => <div data-testid="table-gallery">TableGallery</div>,
}));

vi.mock('./components/TableBoard', () => ({
  default: () => <div data-testid="table-board">TableBoard</div>,
}));

vi.mock('./components/WorkspaceSettings', () => ({
  default: (props: { onClose: () => void }) => (
    <div data-testid="workspace-settings">
      <button onClick={props.onClose}>Close Settings</button>
    </div>
  ),
}));

vi.mock('./components/Onboarding', () => ({
  default: (props: { onComplete: () => void }) => (
    <div data-testid="onboarding">
      <button onClick={props.onComplete}>Complete Onboarding</button>
    </div>
  ),
}));

vi.mock('./components/Auth', () => ({
  default: (props: { onLogin: (token: string, user: UserResponse) => void }) => (
    <div data-testid="auth-form">
      <button
        onClick={() =>
          props.onLogin('test-token', {
            id: 'user-1',
            email: 'test@example.com',
            name: 'Test User',
            organization_id: 'org-1',
            role: 'viewer',
            memberships: [
              {
                id: 'mem-1',
                user_id: 'user-1',
                organization_id: 'org-1',
                organization_name: 'Test Org',
                role: 'viewer',
                settings: { notifications: true },
                created: '2024-01-01T00:00:00Z',
              },
            ],
            settings: { theme: 'light', language: 'en' },
            created: '2024-01-01T00:00:00Z',
            modified: '2024-01-01T00:00:00Z',
          })
        }
      >
        Login
      </button>
    </div>
  ),
}));

vi.mock('./components/Privacy', () => ({
  default: () => <div data-testid="privacy-page">Privacy Policy</div>,
}));

vi.mock('./components/Terms', () => ({
  default: () => <div data-testid="terms-page">Terms of Service</div>,
}));

vi.mock('./components/PWAInstallBanner', () => ({
  default: () => <div data-testid="pwa-banner" />,
}));

vi.mock('./components/CreateOrgModal', () => ({
  default: (props: { isFirstOrg?: boolean; onClose: () => void; onCreate: (data: unknown) => void }) => (
    <div data-testid={props.isFirstOrg ? 'create-org-modal-first' : 'create-org-modal'}>
      <button onClick={props.onClose}>Close</button>
      <button onClick={() => props.onCreate({ name: 'New Org', welcomePageTitle: 'Welcome', welcomePageContent: '' })}>
        Create
      </button>
    </div>
  ),
}));

// Mock fetch
const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

// Mock localStorage
const localStorageMock = (() => {
  let store: Record<string, string> = {};
  return {
    getItem: vi.fn((key: string) => store[key] ?? null),
    setItem: vi.fn((key: string, value: string) => {
      store[key] = value;
    }),
    removeItem: vi.fn((key: string) => {
      delete store[key];
    }),
    clear: () => {
      store = {};
    },
  };
})();
Object.defineProperty(window, 'localStorage', { value: localStorageMock });

// Mock window.history
const mockPushState = vi.fn();
const mockReplaceState = vi.fn();
Object.defineProperty(window, 'history', {
  value: {
    pushState: mockPushState,
    replaceState: mockReplaceState,
  },
  writable: true,
});

// Mock window.location
let mockPathname = '/';
let mockSearch = '';
Object.defineProperty(window, 'location', {
  value: {
    get pathname() {
      return mockPathname;
    },
    set pathname(v: string) {
      mockPathname = v;
    },
    get search() {
      return mockSearch;
    },
    set search(v: string) {
      mockSearch = v;
    },
  },
  writable: true,
});

// Mock confirm
const mockConfirm = vi.fn(() => true);
window.confirm = mockConfirm;

// Helper to render with I18n
function renderWithI18n(component: () => JSX.Element) {
  return render(() => <I18nProvider>{component()}</I18nProvider>);
}

// Mock user data
const mockUser: UserResponse = {
  id: 'user-1',
  email: 'test@example.com',
  name: 'Test User',
  organization_id: 'org-1',
  role: UserRoleViewer,
  memberships: [
    {
      id: 'mem-1',
      user_id: 'user-1',
      organization_id: 'org-1',
      organization_name: 'Test Org',
      role: UserRoleViewer,
      settings: { notifications: true },
      created: '2024-01-01T00:00:00Z',
    },
  ],
  settings: { theme: 'light', language: 'en' },
  created: '2024-01-01T00:00:00Z',
  modified: '2024-01-01T00:00:00Z',
};

const mockNodes: NodeResponse[] = [
  {
    id: 'node-1',
    title: 'Test Page',
    type: 'document',
    content: '# Hello World',
    created: '2024-01-01T00:00:00Z',
    modified: '2024-01-01T00:00:00Z',
  },
  {
    id: 'node-2',
    title: 'Test Table',
    type: 'table',
    properties: [
      { name: 'Name', type: 'text', required: true },
      {
        name: 'Status',
        type: 'select',
        options: [
          { id: 'opt-1', name: 'Todo' },
          { id: 'opt-2', name: 'Done' },
        ],
      },
    ],
    created: '2024-01-01T00:00:00Z',
    modified: '2024-01-01T00:00:00Z',
  },
];

const mockRecords: DataRecordResponse[] = [
  {
    id: 'rec-1',
    data: { Name: 'Record 1', Status: 'Todo' },
    created: '2024-01-01T00:00:00Z',
    modified: '2024-01-01T00:00:00Z',
  },
  {
    id: 'rec-2',
    data: { Name: 'Record 2', Status: 'Done' },
    created: '2024-01-01T00:00:00Z',
    modified: '2024-01-01T00:00:00Z',
  },
];

describe('App', () => {
  beforeEach(() => {
    cleanup();
    vi.clearAllMocks();
    mockFetch.mockReset();
    localStorageMock.clear();
    mockPathname = '/';
    mockSearch = '';
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  describe('Authentication', () => {
    it('shows Auth component when not logged in', async () => {
      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId('auth-form')).toBeTruthy();
      });
    });

    it('shows main app when logged in via localStorage token', async () => {
      localStorageMock.setItem('mddb_token', 'existing-token');

      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser),
          });
        }
        if (url.includes('/nodes')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByText('Test User (viewer)')).toBeTruthy();
      });
    });

    it('extracts OAuth token from URL query params', async () => {
      mockSearch = '?token=oauth-token-from-url';

      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser),
          });
        }
        if (url.includes('/nodes')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(localStorageMock.setItem).toHaveBeenCalledWith('mddb_token', 'oauth-token-from-url');
      });

      await waitFor(() => {
        expect(mockReplaceState).toHaveBeenCalled();
      });
    });

    it('handles logout', async () => {
      localStorageMock.setItem('mddb_token', 'existing-token');

      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser),
          });
        }
        if (url.includes('/nodes')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByText(/logout/i)).toBeTruthy();
      });

      fireEvent.click(screen.getByText(/logout/i));

      await waitFor(() => {
        expect(localStorageMock.removeItem).toHaveBeenCalledWith('mddb_token');
      });

      await waitFor(() => {
        expect(screen.getByTestId('auth-form')).toBeTruthy();
      });
    });

    it('handles login callback from Auth component', async () => {
      mockFetch.mockImplementation((url: string) => {
        if (url.includes('/nodes')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId('auth-form')).toBeTruthy();
      });

      // Click the mock login button
      fireEvent.click(screen.getByText('Login'));

      await waitFor(() => {
        expect(localStorageMock.setItem).toHaveBeenCalledWith('mddb_token', 'test-token');
      });

      await waitFor(() => {
        expect(screen.getByText('Test User (viewer)')).toBeTruthy();
      });
    });
  });

  describe('Static Pages', () => {
    it('shows Privacy page when on /privacy', async () => {
      mockPathname = '/privacy';

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId('privacy-page')).toBeTruthy();
      });
    });

    it('shows Terms page when on /terms', async () => {
      mockPathname = '/terms';

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId('terms-page')).toBeTruthy();
      });
    });
  });

  describe('Node List', () => {
    beforeEach(() => {
      localStorageMock.setItem('mddb_token', 'test-token');
      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser),
          });
        }
        if (url.includes('/nodes')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });
    });

    it('loads and displays nodes in sidebar', async () => {
      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId('sidebar-node-node-1')).toBeTruthy();
        expect(screen.getByTestId('sidebar-node-node-2')).toBeTruthy();
      });
    });

    it('shows welcome message when no node is selected', async () => {
      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByText(/select a node from the sidebar/i)).toBeTruthy();
      });
    });
  });

  describe('Node Selection and Loading', () => {
    beforeEach(() => {
      localStorageMock.setItem('mddb_token', 'test-token');
    });

    it('loads document node when clicked', async () => {
      mockFetch.mockImplementation((url: string, init?: RequestInit) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser),
          });
        }
        if (url === '/api/org-1/nodes') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        if (url === '/api/org-1/nodes/node-1' && (!init || init.method === 'GET' || !init.method)) {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                id: 'node-1',
                title: 'Test Page',
                type: 'document',
                content: '# Hello World',
              }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId('sidebar-node-node-1')).toBeTruthy();
      });

      fireEvent.click(screen.getByTestId('sidebar-node-node-1'));

      await waitFor(() => {
        expect(mockPushState).toHaveBeenCalledWith(null, '', '/org-1/node-1+test-page');
      });

      await waitFor(() => {
        expect(screen.getByDisplayValue('Test Page')).toBeTruthy();
      });
    });

    it('loads table node with records', async () => {
      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser),
          });
        }
        if (url === '/api/org-1/nodes') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        if (url === '/api/org-1/nodes/node-2') {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                id: 'node-2',
                title: 'Test Table',
                type: 'table',
                properties: mockNodes[1]?.properties ?? [],
              }),
          });
        }
        if (url.includes('/tables/node-2/records')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ records: mockRecords }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId('sidebar-node-node-2')).toBeTruthy();
      });

      fireEvent.click(screen.getByTestId('sidebar-node-node-2'));

      await waitFor(() => {
        expect(screen.getByTestId('table-table')).toBeTruthy();
      });
    });
  });

  describe('Node CRUD Operations', () => {
    beforeEach(() => {
      localStorageMock.setItem('mddb_token', 'test-token');
      mockFetch.mockImplementation((url: string, init?: RequestInit) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser),
          });
        }
        if (url === '/api/org-1/nodes' && (!init || init.method === 'GET' || !init.method)) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        if (url === '/api/org-1/nodes' && init?.method === 'POST') {
          const body = JSON.parse(init.body as string);
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                id: 'new-node',
                title: body.title,
                type: body.type,
              }),
          });
        }
        if (url === '/api/org-1/nodes/new-node') {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                id: 'new-node',
                title: 'New Page',
                type: 'document',
                content: '',
              }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });
    });

    it('creates a new document when title is provided', async () => {
      renderWithI18n(() => <App />);

      // Wait for user to be logged in and sidebar to show
      await waitFor(() => {
        expect(screen.getByText('Test User (viewer)')).toBeTruthy();
      });

      // Wait for create page button to be available in welcome view
      await waitFor(() => {
        expect(screen.getByText('Create Page')).toBeTruthy();
      });

      // Fill in title
      fireEvent.input(screen.getByPlaceholderText(/title/i), {
        target: { value: 'New Page' },
      });

      // Click create page button
      fireEvent.click(screen.getByText('Create Page'));

      await waitFor(() => {
        expect(mockFetch).toHaveBeenCalledWith(
          '/api/org-1/nodes',
          expect.objectContaining({
            method: 'POST',
            body: JSON.stringify({ title: 'New Page', type: 'document' }),
          })
        );
      });
    });

    it('shows error when creating node without title', async () => {
      renderWithI18n(() => <App />);

      // Wait for user to be logged in
      await waitFor(() => {
        expect(screen.getByText('Test User (viewer)')).toBeTruthy();
      });

      // Wait for create page button
      await waitFor(() => {
        expect(screen.getByText('Create Page')).toBeTruthy();
      });

      // Click create without entering title
      fireEvent.click(screen.getByText('Create Page'));

      await waitFor(() => {
        expect(screen.getByText(/title is required/i)).toBeTruthy();
      });
    });
  });

  describe('View Mode Switching', () => {
    beforeEach(() => {
      localStorageMock.setItem('mddb_token', 'test-token');
      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser),
          });
        }
        if (url === '/api/org-1/nodes') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        if (url === '/api/org-1/nodes/node-2') {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                id: 'node-2',
                title: 'Test Table',
                type: 'table',
                properties: mockNodes[1]?.properties ?? [],
              }),
          });
        }
        if (url.includes('/tables/node-2/records')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ records: mockRecords }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });
    });

    it('switches between table views', async () => {
      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId('sidebar-node-node-2')).toBeTruthy();
      });

      fireEvent.click(screen.getByTestId('sidebar-node-node-2'));

      await waitFor(() => {
        expect(screen.getByTestId('table-table')).toBeTruthy();
      });

      // Switch to grid view
      fireEvent.click(screen.getByText('Grid'));

      await waitFor(() => {
        expect(screen.getByTestId('table-grid')).toBeTruthy();
      });

      // Switch to gallery view
      fireEvent.click(screen.getByText('Gallery'));

      await waitFor(() => {
        expect(screen.getByTestId('table-gallery')).toBeTruthy();
      });

      // Switch to board view
      fireEvent.click(screen.getByText('Board'));

      await waitFor(() => {
        expect(screen.getByTestId('table-board')).toBeTruthy();
      });
    });
  });

  describe('Settings', () => {
    beforeEach(() => {
      localStorageMock.setItem('mddb_token', 'test-token');
      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser),
          });
        }
        if (url.includes('/nodes')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });
    });

    it('opens settings panel when clicking settings button', async () => {
      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTitle(/settings/i)).toBeTruthy();
      });

      fireEvent.click(screen.getByTitle(/settings/i));

      await waitFor(() => {
        expect(screen.getByTestId('workspace-settings')).toBeTruthy();
      });
    });

    it('closes settings panel', async () => {
      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTitle(/settings/i)).toBeTruthy();
      });

      fireEvent.click(screen.getByTitle(/settings/i));

      await waitFor(() => {
        expect(screen.getByTestId('workspace-settings')).toBeTruthy();
      });

      fireEvent.click(screen.getByText('Close Settings'));

      await waitFor(() => {
        expect(screen.queryByTestId('workspace-settings')).toBeFalsy();
      });
    });
  });

  describe('Organization Management', () => {
    it('shows create org modal for new users without memberships', async () => {
      localStorageMock.setItem('mddb_token', 'test-token');

      const userWithNoMemberships: UserResponse = {
        ...mockUser,
        memberships: [],
      };

      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(userWithNoMemberships),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId('create-org-modal-first')).toBeTruthy();
      });
    });

    it('shows org switcher for users with multiple orgs', async () => {
      localStorageMock.setItem('mddb_token', 'test-token');

      const userWithMultipleOrgs: UserResponse = {
        ...mockUser,
        memberships: [
          {
            id: 'mem-1',
            user_id: 'user-1',
            organization_id: 'org-1',
            organization_name: 'Org 1',
            role: UserRoleViewer,
            settings: { notifications: true },
            created: '2024-01-01T00:00:00Z',
          },
          {
            id: 'mem-2',
            user_id: 'user-1',
            organization_id: 'org-2',
            organization_name: 'Org 2',
            role: UserRoleAdmin,
            settings: { notifications: true },
            created: '2024-01-01T00:00:00Z',
          },
        ],
      };

      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(userWithMultipleOrgs),
          });
        }
        if (url.includes('/nodes')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });

      renderWithI18n(() => <App />);

      await waitFor(() => {
        const select = screen.getByRole('combobox');
        expect(select).toBeTruthy();
      });
    });

    it('opens create org modal when clicking + button', async () => {
      localStorageMock.setItem('mddb_token', 'test-token');

      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser),
          });
        }
        if (url.includes('/nodes')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTitle(/create organization/i)).toBeTruthy();
      });

      fireEvent.click(screen.getByTitle(/create organization/i));

      await waitFor(() => {
        expect(screen.getByTestId('create-org-modal')).toBeTruthy();
      });
    });
  });

  describe('Onboarding', () => {
    it('shows onboarding for admin users who have not completed it', async () => {
      localStorageMock.setItem('mddb_token', 'test-token');

      const adminUser: UserResponse = {
        ...mockUser,
        role: UserRoleAdmin,
        onboarding: { completed: false, step: 'welcome', updated_at: '2024-01-01T00:00:00Z' },
      };

      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(adminUser),
          });
        }
        if (url.includes('/nodes')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId('onboarding')).toBeTruthy();
      });
    });

    it('does not show onboarding for non-admin users', async () => {
      localStorageMock.setItem('mddb_token', 'test-token');

      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser), // role: 'member'
          });
        }
        if (url.includes('/nodes')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByText('Test User (viewer)')).toBeTruthy();
      });

      expect(screen.queryByTestId('onboarding')).toBeFalsy();
    });
  });

  describe('URL Routing', () => {
    beforeEach(() => {
      localStorageMock.setItem('mddb_token', 'test-token');
    });

    it('loads node from URL on mount', async () => {
      // URL with slug uses + separator
      mockPathname = '/org-1/node-1+test-page';

      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser),
          });
        }
        if (url === '/api/org-1/nodes') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        if (url === '/api/org-1/nodes/node-1') {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                id: 'node-1',
                title: 'Test Page',
                type: 'document',
                content: '# Hello',
              }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });

      renderWithI18n(() => <App />);

      // Wait for the node to be loaded from URL
      await waitFor(() => {
        expect(screen.getByDisplayValue('Test Page')).toBeTruthy();
      });
    });
  });

  describe('Error Handling', () => {
    beforeEach(() => {
      localStorageMock.setItem('mddb_token', 'test-token');
    });

    it('displays error message from failed node creation', async () => {
      mockFetch.mockImplementation((url: string, init?: RequestInit) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser),
          });
        }
        if (url === '/api/org-1/nodes' && (!init || init.method === 'GET' || !init.method)) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        // Fail the POST request for creating nodes
        if (url === '/api/org-1/nodes' && init?.method === 'POST') {
          return Promise.resolve({
            ok: false,
            status: 400,
            json: () =>
              Promise.resolve({
                error: { code: 'VALIDATION_ERROR', message: 'Validation failed' },
              }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });

      renderWithI18n(() => <App />);

      // Wait for user to be logged in and welcome screen to show
      await waitFor(() => {
        expect(screen.getByText('Create Page')).toBeTruthy();
      });

      // Fill title and try to create
      fireEvent.input(screen.getByPlaceholderText(/title/i), {
        target: { value: 'Test Title' },
      });
      fireEvent.click(screen.getByText('Create Page'));

      // Wait for error message
      await waitFor(() => {
        expect(screen.getByText(/failed to create/i)).toBeTruthy();
      });
    });
  });

  describe('PWA Banner', () => {
    it('always renders PWA install banner', async () => {
      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId('pwa-banner')).toBeTruthy();
      });
    });
  });
});

describe('slugify', () => {
  // Test the slugify function indirectly through URL updates
  it('creates URL-safe slugs', async () => {
    localStorageMock.setItem('mddb_token', 'test-token');

    mockFetch.mockImplementation((url: string) => {
      if (url === '/api/auth/me') {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              ...mockUser,
            }),
        });
      }
      if (url === '/api/org-1/nodes') {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ nodes: mockNodes }),
        });
      }
      if (url === '/api/org-1/nodes/node-1') {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              id: 'node-1',
              title: 'Hello World Test',
              type: 'document',
              content: '',
            }),
        });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
    });

    renderWithI18n(() => <App />);

    await waitFor(() => {
      expect(screen.getByTestId('sidebar-node-node-1')).toBeTruthy();
    });

    fireEvent.click(screen.getByTestId('sidebar-node-node-1'));

    await waitFor(() => {
      expect(mockPushState).toHaveBeenCalledWith(null, '', '/org-1/node-1+hello-world-test');
    });
  });
});
