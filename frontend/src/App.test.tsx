import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@solidjs/testing-library';
import type { JSX } from 'solid-js';
import App from './App';
import { I18nProvider } from './i18n';
import type { UserResponse, NodeResponse, DataRecordResponse } from './types.gen';
import { WSRoleViewer } from './types.gen';

// Mock CSS modules
vi.mock('./App.module.css', () => ({
  default: {
    app: 'app',
    header: 'header',
    headerTitle: 'headerTitle',
    userInfo: 'userInfo',
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
            org_role: 'member',
            workspace_id: 'ws-1',
            workspace_name: 'Test Workspace',
            workspace_role: 'viewer',
            organizations: [
              {
                id: 'mem-1',
                user_id: 'user-1',
                organization_id: 'org-1',
                organization_name: 'Test Org',
                role: 'member',
                created: 1704067200,
              },
            ],
            workspaces: [
              {
                id: 'wsmem-1',
                user_id: 'user-1',
                workspace_id: 'ws-1',
                workspace_name: 'Default Workspace',
                organization_id: 'org-1',
                role: 'viewer',
                settings: { notifications: true },
                created: 1704067200,
              },
            ],
            settings: { theme: 'light', language: 'en' },
            created: 1704067200,
            modified: 1704067200,
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

vi.mock('./components/UserMenu', () => ({
  default: (props: { user: UserResponse; onLogout: () => void }) => (
    <div data-testid="user-menu">
      <span data-testid="user-info">
        {props.user.name} ({props.user.workspace_role})
      </span>
      <button data-testid="logout-button" onClick={props.onLogout}>
        Logout
      </button>
    </div>
  ),
}));

vi.mock('./components/OrgMenu', () => ({
  default: (props: {
    memberships: { organization_id: string; organization_name?: string }[];
    currentOrgId: string;
    onSwitchOrg: (orgId: string) => void;
    onCreateOrg: () => void;
  }) => (
    <div data-testid="org-menu">
      <button data-testid="org-menu-button" onClick={() => {}}>
        {props.memberships.find((m) => m.organization_id === props.currentOrgId)?.organization_name || 'Workspace'}
      </button>
      <select
        data-testid="org-switcher"
        value={props.currentOrgId}
        onChange={(e) => props.onSwitchOrg((e.target as HTMLSelectElement).value)}
      >
        {props.memberships.map((m) => (
          <option value={m.organization_id}>{m.organization_name || m.organization_id}</option>
        ))}
      </select>
      <button data-testid="create-org-button" onClick={props.onCreateOrg}>
        Create Workspace
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
  org_role: 'member',
  workspace_id: 'ws-1',
  workspace_name: 'Test Workspace',
  workspace_role: WSRoleViewer,
  organizations: [
    {
      id: 'mem-1',
      user_id: 'user-1',
      organization_id: 'org-1',
      organization_name: 'Test Org',
      role: 'member',
      created: 1704067200,
    },
  ],
  workspaces: [
    {
      id: 'wsmem-1',
      user_id: 'user-1',
      workspace_id: 'ws-1',
      workspace_name: 'Test Workspace',
      organization_id: 'org-1',
      role: WSRoleViewer,
      settings: { notifications: true },
      created: 1704067200,
    },
  ],
  settings: { theme: 'light', language: 'en' },
  created: 1704067200,
  modified: 1704067200,
};

const mockNodes: NodeResponse[] = [
  {
    id: 'node-1',
    title: 'Test Page',
    content: '# Hello World',
    created: 1704067200,
    modified: 1704067200,
    has_page: true,
    has_table: false,
  },
  {
    id: 'node-2',
    title: 'Test Table',
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
    created: 1704067200,
    modified: 1704067200,
    has_page: false,
    has_table: true,
  },
];

const mockRecords: DataRecordResponse[] = [
  {
    id: 'rec-1',
    data: { Name: 'Record 1', Status: 'Todo' },
    created: 1704067200,
    modified: 1704067200,
  },
  {
    id: 'rec-2',
    data: { Name: 'Record 2', Status: 'Done' },
    created: 1704067200,
    modified: 1704067200,
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
        if (url === '/api/workspaces/ws-1/nodes') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        if (url === '/api/workspaces/ws-1/nodes/node-1' && (!init || init.method === 'GET' || !init.method)) {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                id: 'node-1',
                title: 'Test Page',
                content: '# Hello World',
                has_page: true,
                has_table: false,
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
        expect(mockPushState).toHaveBeenCalledWith(null, '', '/ws-1+test-workspace/node-1+test-page');
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
        if (url === '/api/workspaces/ws-1/nodes') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        if (url === '/api/workspaces/ws-1/nodes/node-2') {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                id: 'node-2',
                title: 'Test Table',
                properties: mockNodes[1]?.properties ?? [],
                has_page: false,
                has_table: true,
              }),
          });
        }
        if (url.includes('/nodes/node-2/table/records')) {
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
        if (url === '/api/workspaces/ws-1/nodes' && (!init || init.method === 'GET' || !init.method)) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        if (url.includes('/nodes/0/page/create') && init?.method === 'POST') {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                id: 'new-node',
              }),
          });
        }
        if (url === '/api/workspaces/ws-1/nodes/new-node') {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                id: 'new-node',
                title: 'New Page',
                content: '',
                has_page: true,
                has_table: false,
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
          '/api/workspaces/ws-1/nodes/0/page/create',
          expect.objectContaining({
            method: 'POST',
            body: JSON.stringify({ title: 'New Page' }),
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
        if (url === '/api/workspaces/ws-1/nodes') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        if (url === '/api/workspaces/ws-1/nodes/node-2') {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                id: 'node-2',
                title: 'Test Table',
                properties: mockNodes[1]?.properties ?? [],
                has_page: false,
                has_table: true,
              }),
          });
        }
        if (url.includes('/nodes/node-2/table/records')) {
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

  describe('Workspace Management', () => {
    it('auto-creates organization when user has no memberships', async () => {
      localStorageMock.setItem('mddb_token', 'test-token');

      const userWithNoMemberships: UserResponse = {
        ...mockUser,
        organizations: [],
        workspaces: [],
      };

      // After org creation, user will have an org (named after user's first name)
      const userAfterOrgCreation: UserResponse = {
        ...mockUser,
        organizations: [
          {
            id: 'new-membership-1',
            user_id: 'user-1',
            organization_id: 'new-org-1',
            organization_name: "Test's Organization",
            role: 'owner',
            created: 1704067200,
          },
        ],
        organization_id: 'new-org-1',
        workspaces: [],
      };

      let getMeCallCount = 0;
      mockFetch.mockImplementation((url: string, options?: RequestInit) => {
        if (url === '/api/auth/me') {
          getMeCallCount++;
          // First call returns no memberships, subsequent calls return the new org
          if (getMeCallCount === 1) {
            return Promise.resolve({
              ok: true,
              json: () => Promise.resolve(userWithNoMemberships),
            });
          }
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(userAfterOrgCreation),
          });
        }
        if (url === '/api/organizations' && options?.method === 'POST') {
          // Mock org creation (named after user's first name)
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                id: 'new-org-1',
                name: "Test's Organization",
                settings: {},
                created: 1704067200,
                member_count: 1,
                workspace_count: 0,
              }),
          });
        }
        if (url === '/api/auth/switch-org' && options?.method === 'POST') {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                token: 'new-token',
                user: userAfterOrgCreation,
              }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });

      renderWithI18n(() => <App />);

      // Verify that the org creation API is called
      await waitFor(
        () => {
          const createOrgCalls = mockFetch.mock.calls.filter(
            (call: unknown[]) =>
              call[0] === '/api/organizations' && (call[1] as RequestInit | undefined)?.method === 'POST'
          );
          expect(createOrgCalls.length).toBeGreaterThan(0);
        },
        { timeout: 3000 }
      );
    });

    it('shows workspace switcher for users with multiple workspaces', async () => {
      localStorageMock.setItem('mddb_token', 'test-token');

      const userWithMultipleOrgs: UserResponse = {
        ...mockUser,
        organizations: [
          {
            id: 'mem-1',
            user_id: 'user-1',
            organization_id: 'org-1',
            organization_name: 'Org 1',
            role: 'member',
            created: 1704067200,
          },
          {
            id: 'mem-2',
            user_id: 'user-1',
            organization_id: 'org-2',
            organization_name: 'Org 2',
            role: 'admin',
            created: 1704067200,
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
        expect(screen.getByTestId('org-menu')).toBeTruthy();
      });
    });

    it('opens create workspace modal when clicking + button', async () => {
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
        expect(screen.getByTestId('create-org-button')).toBeTruthy();
      });

      fireEvent.click(screen.getByTestId('create-org-button'));

      await waitFor(() => {
        expect(screen.getByTestId('create-org-modal')).toBeTruthy();
      });
    });
  });

  describe('Onboarding', () => {
    it('does not show onboarding on initial load', async () => {
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
        expect(screen.getByText('Test User (viewer)')).toBeTruthy();
      });

      // Onboarding is only shown after org creation, not on initial load
      expect(screen.queryByTestId('onboarding')).toBeFalsy();
    });
  });

  describe('URL Routing', () => {
    beforeEach(() => {
      localStorageMock.setItem('mddb_token', 'test-token');
    });

    it('loads node from URL on mount', async () => {
      // URL with slug uses + separator
      mockPathname = '/ws-1+test-workspace/node-1+test-page';

      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser),
          });
        }
        if (url === '/api/workspaces/ws-1/nodes') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        if (url === '/api/workspaces/ws-1/nodes/node-1') {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                id: 'node-1',
                title: 'Test Page',
                content: '# Hello',
                has_page: true,
                has_table: false,
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
        if (url === '/api/workspaces/ws-1/nodes' && (!init || init.method === 'GET' || !init.method)) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        // Fail the POST request for creating pages
        if (url.includes('/nodes/0/page/create') && init?.method === 'POST') {
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
      if (url === '/api/workspaces/ws-1/nodes') {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ nodes: mockNodes }),
        });
      }
      if (url === '/api/workspaces/ws-1/nodes/node-1') {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              id: 'node-1',
              title: 'Hello World Test',
              content: '',
              has_page: true,
              has_table: false,
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
      expect(mockPushState).toHaveBeenCalledWith(null, '', '/ws-1+test-workspace/node-1+hello-world-test');
    });
  });
});
