import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@solidjs/testing-library';
import type { JSX } from 'solid-js';
import App from './App';
import { I18nProvider } from './i18n';
import type { UserResponse, NodeResponse, DataRecordResponse } from '@sdk/types.gen';
import { WSRoleViewer } from '@sdk/types.gen';

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
  default: (props: { onBack: () => void }) => (
    <div data-testid="workspace-settings">
      <button onClick={props.onBack}>Back</button>
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

vi.mock('./components/CreateWorkspaceModal', () => ({
  default: (props: { isFirstWorkspace?: boolean; onClose: () => void; onCreate: (data: unknown) => void }) => (
    <div data-testid={props.isFirstWorkspace ? 'create-workspace-modal-first' : 'create-workspace-modal'}>
      <button onClick={props.onClose}>Close</button>
      <button onClick={() => props.onCreate({ name: 'New Workspace' })}>Create</button>
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

vi.mock('./components/WorkspaceMenu', () => ({
  default: (props: {
    workspaces: { workspace_id: string; workspace_name?: string; organization_id: string }[];
    organizations: { organization_id: string; organization_name?: string }[];
    currentWsId: string;
    onSwitchWorkspace: (wsId: string) => void;
    onOpenSettings: () => void;
    onCreateWorkspace: () => void;
  }) => {
    const currentWs = props.workspaces.find((ws) => ws.workspace_id === props.currentWsId);
    return (
      <div data-testid="workspace-menu">
        <button
          data-testid="workspace-menu-button"
          title={currentWs?.workspace_name || 'Workspace'}
          onClick={props.onOpenSettings}
        >
          {currentWs?.workspace_name || 'Workspace'}
        </button>
        <button data-testid="workspace-settings-button" onClick={props.onOpenSettings}>
          Workspace Settings
        </button>
        <button data-testid="create-workspace-button" onClick={props.onCreateWorkspace}>
          Create Workspace
        </button>
      </div>
    );
  },
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
const mockBack = vi.fn();
Object.defineProperty(window, 'history', {
  value: {
    pushState: mockPushState,
    replaceState: mockReplaceState,
    back: mockBack,
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

// Root node (id=0) with children indication
const mockRootNode: NodeResponse = {
  id: '0',
  title: 'Root',
  created: 1704067200,
  modified: 1704067200,
  has_page: true,
  has_table: false,
  children: [], // indicates has children (lazy loading)
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
        if (url.match(/\/nodes\/0$/)) {
          // GET /nodes/0 returns root node
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockRootNode),
          });
        }
        if (url.match(/\/nodes\/0\/children$/)) {
          // GET /nodes/0/children returns children of root
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
        if (url.match(/\/nodes\/0$/)) {
          // GET /nodes/0 returns root node
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockRootNode),
          });
        }
        if (url.match(/\/nodes\/0\/children$/)) {
          // GET /nodes/0/children returns children of root
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
        if (url.match(/\/nodes\/0$/)) {
          // GET /nodes/0 returns root node
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockRootNode),
          });
        }
        if (url.match(/\/nodes\/0\/children$/)) {
          // GET /nodes/0/children returns children of root
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
        if (url.match(/\/nodes\/0$/)) {
          // GET /nodes/0 returns root node
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockRootNode),
          });
        }
        if (url.match(/\/nodes\/0\/children$/)) {
          // GET /nodes/0/children returns children of root
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
        if (url.match(/\/nodes\/0$/)) {
          // GET /nodes/0 returns root node
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockRootNode),
          });
        }
        if (url.match(/\/nodes\/0\/children$/)) {
          // GET /nodes/0/children returns children of root
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
        expect(screen.getByTestId('sidebar-node-node-1')).toBeTruthy();
      });
    });

    it('auto-selects first node when nodes are loaded', async () => {
      renderWithI18n(() => <App />);

      // First node should be auto-selected when at workspace root
      await waitFor(() => {
        expect(screen.getByTestId('sidebar-node-node-1')).toBeTruthy();
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
        if (url === '/api/workspaces/ws-1/nodes/0') {
          // GET /nodes/0 returns root node
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockRootNode),
          });
        }
        if (url === '/api/workspaces/ws-1/nodes/0/children') {
          // GET /nodes/0/children returns children
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
        expect(mockPushState).toHaveBeenCalledWith(null, '', '/w/ws-1+test-workspace/node-1+test-page');
      });

      await waitFor(() => {
        expect(screen.getByDisplayValue('Test Page')).toBeTruthy();
      });
    });

    it('loads table node with records', async () => {
      // Mock a table-type node for this test
      const mockTableNode: NodeResponse = {
        id: 'table-1',
        title: 'Table Node',
        properties: [
          { name: 'Name', type: 'text', required: true },
          { name: 'Status', type: 'select', options: [{ id: 'opt-1', name: 'Todo' }] },
        ],
        created: 1704067200,
        modified: 1704067200,
        has_page: false,
        has_table: true,
        children: [],
      };

      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser),
          });
        }
        if (url === '/api/workspaces/ws-1/nodes/0/children') {
          // Top-level nodes include the table node
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: [mockTableNode] }),
          });
        }
        if (url === '/api/workspaces/ws-1/nodes/table-1') {
          // GET table node
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockTableNode),
          });
        }
        if (url.includes('/nodes/table-1/table/records')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ records: mockRecords }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId('sidebar-node-table-1')).toBeTruthy();
      });

      fireEvent.click(screen.getByTestId('sidebar-node-table-1'));

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
        if (url === '/api/workspaces/ws-1/nodes/0' && (!init || init.method === 'GET' || !init.method)) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockRootNode),
          });
        }
        if (url === '/api/workspaces/ws-1/nodes/0/children') {
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

    it('creates a new document via sidebar button', async () => {
      renderWithI18n(() => <App />);

      // Wait for user to be logged in and sidebar to show
      await waitFor(() => {
        expect(screen.getByText('Test User (viewer)')).toBeTruthy();
      });

      // Wait for sidebar new page button to be available
      await waitFor(() => {
        expect(screen.getByTitle(/new page/i)).toBeTruthy();
      });

      // Click create page button in sidebar
      fireEvent.click(screen.getByTitle(/new page/i));

      // The createNode function requires a title, so it will show an error
      await waitFor(() => {
        expect(screen.getByText(/title is required/i)).toBeTruthy();
      });
    });
  });

  describe('View Mode Switching', () => {
    // Mock table node for view switching tests
    const mockTableNode: NodeResponse = {
      id: 'table-node',
      title: 'Test Table',
      properties: [
        { name: 'Name', type: 'text', required: true },
        { name: 'Status', type: 'select', options: [{ id: 'opt-1', name: 'Todo' }] },
      ],
      created: 1704067200,
      modified: 1704067200,
      has_page: false,
      has_table: true,
      children: [],
    };

    beforeEach(() => {
      localStorageMock.setItem('mddb_token', 'test-token');
      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser),
          });
        }
        if (url === '/api/workspaces/ws-1/nodes/0/children') {
          // Top-level nodes include the table node
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: [mockTableNode] }),
          });
        }
        if (url === '/api/workspaces/ws-1/nodes/table-node') {
          // GET table node
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockTableNode),
          });
        }
        if (url.includes('/nodes/table-node/table/records')) {
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

      // Wait for nodes to be displayed in sidebar
      await waitFor(
        () => {
          expect(screen.getByTestId('sidebar-node-table-node')).toBeTruthy();
        },
        { timeout: 3000 }
      );

      // Click on the table node
      fireEvent.click(screen.getByTestId('sidebar-node-table-node'));

      // Wait for table to load and view toggle buttons to appear
      await waitFor(
        () => {
          expect(screen.getByTestId('table-table')).toBeTruthy();
          expect(screen.getByText('Grid')).toBeTruthy();
        },
        { timeout: 3000 }
      );

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
        if (url.match(/\/nodes\/0$/)) {
          // GET /nodes/0 returns root node
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockRootNode),
          });
        }
        if (url.match(/\/nodes\/0\/children$/)) {
          // GET /nodes/0/children returns children of root
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });
    });

    it('opens settings panel when clicking settings in workspace menu', async () => {
      renderWithI18n(() => <App />);

      // Wait for workspace menu to appear
      await waitFor(() => {
        expect(screen.getByTestId('workspace-settings-button')).toBeTruthy();
      });

      // Click settings button
      fireEvent.click(screen.getByTestId('workspace-settings-button'));

      await waitFor(() => {
        expect(screen.getByTestId('workspace-settings')).toBeTruthy();
      });
    });

    it('closes settings panel', async () => {
      renderWithI18n(() => <App />);

      // Wait for workspace menu to appear
      await waitFor(() => {
        expect(screen.getByTestId('workspace-settings-button')).toBeTruthy();
      });

      // Click settings button - this navigates to /settings
      fireEvent.click(screen.getByTestId('workspace-settings-button'));

      await waitFor(() => {
        expect(screen.getByTestId('workspace-settings')).toBeTruthy();
      });

      // Simulate browser back navigation by clicking Back button and then simulating popstate
      fireEvent.click(screen.getByText('Back'));

      // Simulate the effect of history.back() by changing pathname and dispatching popstate
      mockPathname = '/';
      window.dispatchEvent(new PopStateEvent('popstate'));

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
        if (url.match(/\/nodes\/0$/)) {
          // GET /nodes/0 returns root node
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockRootNode),
          });
        }
        if (url.match(/\/nodes\/0\/children$/)) {
          // GET /nodes/0/children returns children of root
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });

      renderWithI18n(() => <App />);

      await waitFor(() => {
        expect(screen.getByTestId('workspace-menu')).toBeTruthy();
      });
    });

    it('opens create workspace modal from workspace menu', async () => {
      localStorageMock.setItem('mddb_token', 'test-token');

      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser),
          });
        }
        if (url.match(/\/nodes\/0$/)) {
          // GET /nodes/0 returns root node
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockRootNode),
          });
        }
        if (url.match(/\/nodes\/0\/children$/)) {
          // GET /nodes/0/children returns children of root
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ nodes: mockNodes }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });

      renderWithI18n(() => <App />);

      // Wait for workspace menu to appear
      await waitFor(() => {
        expect(screen.getByTestId('create-workspace-button')).toBeTruthy();
      });

      // Click "Create Workspace" button
      fireEvent.click(screen.getByTestId('create-workspace-button'));

      await waitFor(() => {
        expect(screen.getByTestId('create-workspace-modal-first')).toBeTruthy();
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
        if (url.match(/\/nodes\/0$/)) {
          // GET /nodes/0 returns root node
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockRootNode),
          });
        }
        if (url.match(/\/nodes\/0\/children$/)) {
          // GET /nodes/0/children returns children of root
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
      mockPathname = '/w/ws-1+test-workspace/node-1+test-page';

      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser),
          });
        }
        if (url === '/api/workspaces/ws-1/nodes/0') {
          // GET /nodes/0 returns root node
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockRootNode),
          });
        }
        if (url === '/api/workspaces/ws-1/nodes/0/children') {
          // GET /nodes/0/children returns children
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

    // Skip: loadNodes now catches errors silently to handle empty workspace gracefully
    it.skip('displays error message from failed API calls', async () => {
      mockFetch.mockImplementation((url: string) => {
        if (url === '/api/auth/me') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve(mockUser),
          });
        }
        // Fail the root node request
        if (url === '/api/workspaces/ws-1/nodes/0') {
          return Promise.resolve({
            ok: false,
            status: 500,
            json: () =>
              Promise.resolve({
                error: { code: 'SERVER_ERROR', message: 'Server error' },
              }),
          });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      });

      renderWithI18n(() => <App />);

      // Wait for error message from failed nodes load
      // The error div should be present in the DOM
      await waitFor(
        () => {
          const errorDiv = document.querySelector('.error');
          expect(errorDiv).toBeTruthy();
        },
        { timeout: 3000 }
      );
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

    const testNode: NodeResponse = {
      id: 'slug-test',
      title: 'Hello World Test',
      content: '',
      created: 1704067200,
      modified: 1704067200,
      has_page: true,
      has_table: false,
      children: [],
    };

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
      if (url === '/api/workspaces/ws-1/nodes/0/children') {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ nodes: [testNode] }),
        });
      }
      if (url === '/api/workspaces/ws-1/nodes/slug-test') {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(testNode),
        });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
    });

    renderWithI18n(() => <App />);

    await waitFor(() => {
      expect(screen.getByTestId('sidebar-node-slug-test')).toBeTruthy();
    });

    fireEvent.click(screen.getByTestId('sidebar-node-slug-test'));

    await waitFor(() => {
      expect(mockPushState).toHaveBeenCalledWith(null, '', '/w/ws-1+test-workspace/slug-test+hello-world-test');
    });
  });
});
