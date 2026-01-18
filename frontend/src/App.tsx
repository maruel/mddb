import { createSignal, createEffect, For, Show, onMount, onCleanup } from 'solid-js';
import SidebarNode from './components/SidebarNode';
import MarkdownPreview from './components/MarkdownPreview';
import DatabaseTable from './components/DatabaseTable';
import DatabaseGrid from './components/DatabaseGrid';
import DatabaseGallery from './components/DatabaseGallery';
import DatabaseBoard from './components/DatabaseBoard';
import WorkspaceSettings from './components/WorkspaceSettings';
import Auth from './components/Auth';
import { debounce } from './utils/debounce';
import type {
  Node,
  DataRecord,
  Commit,
  User,
  ListNodesResponse,
  ListRecordsResponse,
  GetPageHistoryResponse,
} from './types';
import styles from './App.module.css';

const slugify = (text: string) => {
  return text
    .toString()
    .toLowerCase()
    .trim()
    .replace(/\s+/g, '-')
    .replace(/[^\w-]+/g, '')
    .replace(/--+/g, '-');
};

export default function App() {
  const [user, setUser] = createSignal<User | null>(null);
  const [token, setToken] = createSignal<string | null>(localStorage.getItem('mddb_token'));
  const [nodes, setNodes] = createSignal<Node[]>([]);
  const [records, setRecords] = createSignal<DataRecord[]>([]);
  const [selectedNodeId, setSelectedNodeId] = createSignal<string | null>(null);
  const [showSettings, setShowSettings] = createSignal(false);
  const [viewMode, setViewMode] = createSignal<'table' | 'grid' | 'gallery' | 'board'>('table');
  const [title, setTitle] = createSignal('');
  const [content, setContent] = createSignal('');
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [hasUnsavedChanges, setHasUnsavedChanges] = createSignal(false);
  const [autoSaveStatus, setAutoSaveStatus] = createSignal<'idle' | 'saving' | 'saved'>('idle');

  // History state
  const [showHistory, setShowHistory] = createSignal(false);
  const [history, setHistory] = createSignal<Commit[]>([]);

  // Pagination
  const [hasMore, setHasMore] = createSignal(false);
  const PAGE_SIZE = 50;

  // Helper for authenticated fetch
  const authFetch = async (url: string, options: RequestInit = {}) => {
    let finalUrl = url;
    // Automatically prepend organization ID for data-acting API calls
    if (
      url.startsWith('/api/') &&
      !url.startsWith('/api/auth/') &&
      !url.startsWith('/api/health')
    ) {
      const orgID = user()?.organization_id;
      if (orgID) {
        // Convert /api/nodes to /api/{orgID}/nodes
        finalUrl = `/api/${orgID}${url.substring(4)}`;
      }
    }

    const headers = {
      ...options.headers,
      Authorization: `Bearer ${token()}`,
    };
    const res = await fetch(finalUrl, { ...options, headers });
    if (res.status === 401) {
      logout();
      throw new Error('Session expired');
    }
    return res;
  };

  const logout = () => {
    localStorage.removeItem('mddb_token');
    setToken(null);
    setUser(null);
    window.history.pushState(null, '', '/');
  };

  const handleLogin = (newToken: string, userData: User) => {
    localStorage.setItem('mddb_token', newToken);
    setToken(newToken);
    setUser(userData);
  };

  async function switchOrg(orgId: string, redirect = true) {
    try {
      setLoading(true);
      const res = await authFetch('/api/auth/switch-org', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ org_id: orgId }),
      });
      const data = await res.json();
      handleLogin(data.token, data.user);
      setSelectedNodeId(null);
      if (redirect) {
        window.history.pushState(null, '', '/');
      }
      await loadNodes();
    } catch (err) {
      setError('Failed to switch organization: ' + err);
      throw err; // Propagate error for callers
    } finally {
      setLoading(false);
    }
  }

  // Debounced auto-save function
  // eslint-disable-next-line solid/reactivity
  const debouncedAutoSave = debounce(async () => {
    const nodeId = selectedNodeId();
    if (!nodeId || !hasUnsavedChanges()) return;

    try {
      setAutoSaveStatus('saving');
      await authFetch(`/api/pages/${nodeId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: title(), content: content() }),
      });
      setHasUnsavedChanges(false);
      setAutoSaveStatus('saved');

      // Update URL if title changed
      const slug = slugify(title());
      const orgId = user()?.organization_id;
      if (orgId) {
        const currentPath = window.location.pathname;
        const newPath = `/${orgId}/${nodeId}${slug ? '-' + slug : ''}`;
        if (currentPath !== newPath) {
          window.history.replaceState(null, '', newPath);
        }
      }

      setTimeout(() => {
        if (autoSaveStatus() === 'saved') {
          setAutoSaveStatus('idle');
        }
      }, 2000);
    } catch (err) {
      setError('Auto-save failed: ' + err);
      setAutoSaveStatus('idle');
    }
  }, 2000);

  // Load user on mount
  createEffect(() => {
    // Check URL for OAuth token
    const urlParams = new URLSearchParams(window.location.search);
    const urlToken = urlParams.get('token');
    if (urlToken) {
      localStorage.setItem('mddb_token', urlToken);
      setToken(urlToken);
      // Clean up URL query params but keep pathname
      window.history.replaceState({}, document.title, window.location.pathname);
    }

    const t = token();
    const u = user();
    if (t && !u) {
      const fetchUser = async () => {
        try {
          const res = await authFetch('/api/auth/me');
          if (res.ok) {
            const data = await res.json();
            setUser(data);
          }
        } catch (err) {
          console.error('Failed to load user', err);
        }
      };
      fetchUser();
    }
  });

  // Handle browser back/forward and initial URL
  const handlePopState = async () => {
    const path = window.location.pathname;

    // Check for /orgID/nodeID
    const matchWithOrg = path.match(/^\/([^/]+)\/(\d+)(?:-.*)?$/);
    if (matchWithOrg) {
      const orgId = matchWithOrg[1];
      const nodeId = matchWithOrg[2];

      // If we are logged in but in wrong org, switch
      if (user() && user()?.organization_id !== orgId) {
        try {
          // Check if we are member of this org
          const isMember = user()?.memberships?.some((m) => m.organization_id === orgId);
          if (isMember) {
            await switchOrg(orgId, false); // Don't redirect to /
          } else {
            setError(`You do not have access to organization ${orgId}`);
            return;
          }
        } catch (e) {
          return;
        }
      }

      if (nodeId !== selectedNodeId()) {
        loadNode(nodeId, false);
      }
      return;
    }

    setSelectedNodeId(null);
    setTitle('');
    setContent('');
  };

  onMount(() => {
    window.addEventListener('popstate', handlePopState);
    onCleanup(() => window.removeEventListener('popstate', handlePopState));
  });

  // Load nodes and initial route when user is available
  createEffect(() => {
    if (user()) {
      loadNodes();
      // Check URL
      handlePopState();
    }
  });

  async function loadNodes() {
    try {
      setLoading(true);
      const res = await authFetch('/api/nodes');
      const data = (await res.json()) as ListNodesResponse;
      setNodes((data.nodes?.filter(Boolean) as Node[]) || []);
      setError(null);
    } catch (err) {
      setError('Failed to load nodes: ' + err);
    } finally {
      setLoading(false);
    }
  }

  async function loadNode(id: string, pushState = true) {
    try {
      setLoading(true);
      setShowHistory(false);
      const res = await authFetch(`/api/nodes/${id}`);
      if (!res.ok) throw new Error('Node not found');
      const nodeData = await res.json();

      setSelectedNodeId(nodeData.id);
      setTitle(nodeData.title);
      setContent(nodeData.content || '');
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);

      // Update URL to include OrgID
      const slug = slugify(nodeData.title);
      const orgId = user()?.organization_id;
      if (orgId) {
        const url = `/${orgId}/${nodeData.id}${slug ? '-' + slug : ''}`;
        if (pushState) {
          if (window.location.pathname !== url) {
            window.history.pushState(null, '', url);
          }
        } else {
          // Canonicalize URL
          if (window.location.pathname !== url) {
            window.history.replaceState(null, '', url);
          }
        }
      }

      // If it's a database or hybrid, load records
      if (nodeData.type === 'database' || nodeData.type === 'hybrid') {
        const recordsRes = await authFetch(
          `/api/databases/${id}/records?offset=0&limit=${PAGE_SIZE}`
        );
        const recordsData = (await recordsRes.json()) as ListRecordsResponse;
        const loadedRecords = (recordsData.records || []) as DataRecord[];
        setRecords(loadedRecords);
        setHasMore(loadedRecords.length === PAGE_SIZE);
      } else {
        setRecords([]);
        setHasMore(false);
      }
    } catch (err) {
      setError('Failed to load node: ' + err);
    } finally {
      setLoading(false);
    }
  }

  async function loadHistory(nodeId: string) {
    if (showHistory()) {
      setShowHistory(false);
      return;
    }

    try {
      setLoading(true);
      const res = await authFetch(`/api/pages/${nodeId}/history`);
      const data = (await res.json()) as GetPageHistoryResponse;
      setHistory((data.history?.filter(Boolean) as Commit[]) || []);
      setShowHistory(true);
    } catch (err) {
      setError('Failed to load history: ' + err);
    } finally {
      setLoading(false);
    }
  }

  async function loadVersion(nodeId: string, hash: string) {
    if (
      !confirm(
        'This will replace current editor content with the selected version. Unsaved changes will be lost. Continue?'
      )
    )
      return;

    try {
      setLoading(true);
      const res = await authFetch(`/api/pages/${nodeId}/history/${hash}`);
      const data = await res.json();
      setContent(data.content);
      setHasUnsavedChanges(true); // Mark as modified
      setShowHistory(false);
    } catch (err) {
      setError('Failed to load version: ' + err);
    } finally {
      setLoading(false);
    }
  }

  async function createNode(type: 'document' | 'database' = 'document') {
    if (!title().trim()) {
      setError('Title is required');
      return;
    }

    try {
      setLoading(true);
      const res = await authFetch('/api/nodes', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: title(), type }),
      });
      const newNode = await res.json();
      await loadNodes();
      loadNode(newNode.id);
      setTitle('');
      setContent('');
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);
    } catch (err) {
      setError('Failed to create node: ' + err);
    } finally {
      setLoading(false);
    }
  }

  async function saveNode() {
    const nodeId = selectedNodeId();
    if (!nodeId) return;

    try {
      setLoading(true);
      await authFetch(`/api/pages/${nodeId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: title(), content: content() }),
      });
      await loadNodes();
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);

      // Update URL if title changed
      const slug = slugify(title());
      const currentPath = window.location.pathname;
      const newPath = `/${nodeId}${slug ? '-' + slug : ''}`;
      if (currentPath !== newPath) {
        window.history.replaceState(null, '', newPath);
      }
    } catch (err) {
      setError('Failed to save node: ' + err);
    } finally {
      setLoading(false);
    }
  }

  async function deleteCurrentNode() {
    const nodeId = selectedNodeId();
    if (!nodeId) return;

    if (!confirm('Are you sure you want to delete this node?')) return;

    try {
      setLoading(true);
      await authFetch(`/api/pages/${nodeId}`, { method: 'DELETE' });
      await loadNodes();
      setSelectedNodeId(null);
      setTitle('');
      setContent('');
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);
      window.history.pushState(null, '', '/');
    } catch (err) {
      setError('Failed to delete node: ' + err);
    } finally {
      setLoading(false);
    }
  }

  const handleNodeClick = (node: Node) => {
    loadNode(node.id);
  };

  const getBreadcrumbs = (nodeId: string | null): Node[] => {
    if (!nodeId) return [];
    const path: Node[] = [];

    const findPath = (currentNodes: Node[], targetId: string): boolean => {
      for (const node of currentNodes) {
        if (node.id === targetId) {
          path.push(node);
          return true;
        }
        if (node.children && findPath(node.children, targetId)) {
          path.unshift(node);
          return true;
        }
      }
      return false;
    };

    findPath(nodes(), nodeId);
    return path;
  };

  async function handleAddRecord(data: Record<string, unknown>) {
    const nodeId = selectedNodeId();
    if (!nodeId) return;

    try {
      setLoading(true);
      const res = await authFetch(`/api/databases/${nodeId}/records`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ data }),
      });

      if (!res.ok) {
        setError('Failed to create record');
        return;
      }

      // Reload records
      loadNode(nodeId);
      setError(null);
    } catch (err) {
      setError('Failed to add record: ' + err);
    } finally {
      setLoading(false);
    }
  }

  async function handleDeleteRecord(recordId: string) {
    const nodeId = selectedNodeId();
    if (!nodeId) return;

    if (!confirm('Delete this record?')) return;

    try {
      setLoading(true);
      await authFetch(`/api/databases/${nodeId}/records/${recordId}`, { method: 'DELETE' });
      loadNode(nodeId);
      setError(null);
    } catch (err) {
      setError('Failed to delete record: ' + err);
    } finally {
      setLoading(false);
    }
  }

  async function loadMoreRecords() {
    const nodeId = selectedNodeId();
    if (!nodeId || loading()) return;

    try {
      setLoading(true);
      const offset = records().length;
      const res = await authFetch(
        `/api/databases/${nodeId}/records?offset=${offset}&limit=${PAGE_SIZE}`
      );
      const data = await res.json();
      const newRecords = data.records || [];
      setRecords([...records(), ...newRecords]);
      setHasMore(newRecords.length === PAGE_SIZE);
    } catch (err) {
      setError('Failed to load more records: ' + err);
    } finally {
      setLoading(false);
    }
  }

  return (
    <Show when={user()} fallback={<Auth onLogin={handleLogin} />}>
      <div class={styles.app}>
        <header class={styles.header}>
          <div class={styles.headerTitle}>
            <h1>mddb</h1>
            <p>A seamless markdown-based document and database system</p>
          </div>
          <div class={styles.userInfo}>
            <Show when={(user()?.memberships?.length ?? 0) > 1}>
              <select
                class={styles.orgSwitcher}
                value={user()?.organization_id}
                onChange={(e) => switchOrg(e.target.value)}
              >
                <For each={user()?.memberships}>
                  {(m) => (
                    <option value={m.organization_id}>
                      {m.organization_name || m.organization_id}
                    </option>
                  )}
                </For>
              </select>
            </Show>
            <Show when={user()?.memberships?.length === 1}>
              <span class={styles.orgName}>
                {user()?.memberships?.[0]?.organization_name || 'My Org'}
              </span>
            </Show>
            <span>
              {user()?.name} ({user()?.role})
            </span>
            <button onClick={logout} class={styles.logoutButton}>
              Logout
            </button>
          </div>
        </header>

        <div class={styles.container}>
          <aside class={styles.sidebar}>
            <div class={styles.sidebarHeader}>
              <h2>Workspace</h2>
              <div class={styles.sidebarActions}>
                <button
                  onClick={() => {
                    setShowSettings(true);
                    setSelectedNodeId(null);
                  }}
                  title="Workspace Settings"
                >
                  ⚙
                </button>
                <button
                  onClick={() => {
                    setShowSettings(false);
                    createNode('document');
                  }}
                  title="New Page"
                >
                  +P
                </button>
                <button
                  onClick={() => {
                    setShowSettings(false);
                    createNode('database');
                  }}
                  title="New Database"
                >
                  +D
                </button>
              </div>
            </div>

            <Show when={loading() && nodes().length === 0} fallback={null}>
              <p class={styles.loading}>Loading...</p>
            </Show>

            <ul class={styles.pageList}>
              <For each={nodes()}>
                {(node) => (
                  <SidebarNode
                    node={node}
                    selectedId={selectedNodeId()}
                    onSelect={handleNodeClick}
                    depth={0}
                  />
                )}
              </For>
            </ul>
          </aside>

          <main class={styles.main}>
            <Show when={showSettings() && user() && token()}>
              <WorkspaceSettings
                user={user() as User}
                token={token() as string}
                onClose={() => setShowSettings(false)}
              />
            </Show>

            <Show when={!showSettings()}>
              <Show when={selectedNodeId()}>
                <nav class={styles.breadcrumbs}>
                  <For each={getBreadcrumbs(selectedNodeId())}>
                    {(crumb, i) => (
                      <>
                        <Show when={i() > 0}>
                          <span class={styles.breadcrumbSeparator}>/</span>
                        </Show>
                        <span class={styles.breadcrumbItem} onClick={() => handleNodeClick(crumb)}>
                          {crumb.title}
                        </span>
                      </>
                    )}
                  </For>
                </nav>
              </Show>

              <Show when={error()} fallback={null}>
                <div class={styles.error}>{error()}</div>
              </Show>

              <Show
                when={selectedNodeId()}
                fallback={
                  <div class={styles.welcome}>
                    <h2>Welcome to mddb</h2>
                    <p>Select a node from the sidebar or create a new one to get started.</p>
                    <div class={styles.createForm}>
                      <input
                        type="text"
                        placeholder="Title"
                        value={title()}
                        onInput={(e) => setTitle(e.target.value)}
                        class={styles.titleInput}
                      />
                      <div class={styles.welcomeActions}>
                        <button onClick={() => createNode('document')} class={styles.createButton}>
                          Create Page
                        </button>
                        <button onClick={() => createNode('database')} class={styles.createButton}>
                          Create Database
                        </button>
                      </div>
                    </div>
                  </div>
                }
              >
                <div class={styles.editor}>
                  <div class={styles.editorHeader}>
                    <input
                      type="text"
                      placeholder="Title"
                      value={title()}
                      onInput={(e) => {
                        setTitle(e.target.value);
                        setHasUnsavedChanges(true);
                        debouncedAutoSave();
                      }}
                      class={styles.titleInput}
                    />
                    <div class={styles.editorStatus}>
                      <Show when={hasUnsavedChanges()}>
                        <span class={styles.unsavedIndicator}>● Unsaved</span>
                      </Show>
                      <Show when={autoSaveStatus() === 'saving'}>
                        <span class={styles.savingIndicator}>⟳ Saving...</span>
                      </Show>
                      <Show when={autoSaveStatus() === 'saved'}>
                        <span class={styles.savedIndicator}>✓ Saved</span>
                      </Show>
                    </div>
                    <div class={styles.editorActions}>
                      <button
                        onClick={() => {
                          const id = selectedNodeId();
                          if (id) loadHistory(id);
                        }}
                        disabled={loading()}
                      >
                        {showHistory() ? 'Hide History' : 'History'}
                      </button>
                      <button onClick={saveNode} disabled={loading()}>
                        {loading() ? 'Saving...' : 'Save'}
                      </button>
                      <button onClick={deleteCurrentNode} disabled={loading()}>
                        Delete
                      </button>
                    </div>
                  </div>

                  <Show when={showHistory()}>
                    <div class={styles.historyPanel}>
                      <h3>Version History</h3>
                      <ul class={styles.historyList}>
                        <For each={history()}>
                          {(commit) => (
                            <li
                              class={styles.historyItem}
                              onClick={() => {
                                const id = selectedNodeId();
                                if (id) loadVersion(id, commit.hash);
                              }}
                            >
                              <div class={styles.historyMeta}>
                                <span class={styles.historyDate}>
                                  {new Date(commit.timestamp).toLocaleString()}
                                </span>
                                <span class={styles.historyHash}>
                                  {commit.hash.substring(0, 7)}
                                </span>
                              </div>
                              <div class={styles.historyMessage}>{commit.message}</div>
                            </li>
                          )}
                        </For>
                        <Show when={history().length === 0}>
                          <li class={styles.historyItem}>No history available</li>
                        </Show>
                      </ul>
                    </div>
                  </Show>

                  <div class={styles.nodeContent}>
                    {/* Always show markdown content if it exists or if node is document/hybrid */}
                    <Show
                      when={nodes().find((n) => n.id === selectedNodeId())?.type !== 'database'}
                    >
                      <div class={styles.editorContent}>
                        <textarea
                          value={content()}
                          onInput={(e) => {
                            setContent(e.target.value);
                            setHasUnsavedChanges(true);
                            debouncedAutoSave();
                          }}
                          placeholder="Write your content in markdown..."
                          class={styles.contentInput}
                        />
                        <MarkdownPreview content={content()} orgId={user()?.organization_id} />
                      </div>
                    </Show>

                    {/* Show database table if node is database or hybrid */}
                    <Show
                      when={nodes().find((n) => n.id === selectedNodeId())?.type !== 'document'}
                    >
                      <div class={styles.databaseView}>
                        <div class={styles.databaseHeader}>
                          <h3>Database Records</h3>
                          <div class={styles.viewToggle}>
                            <button
                              classList={{ [`${styles.active}`]: viewMode() === 'table' }}
                              onClick={() => setViewMode('table')}
                            >
                              Table
                            </button>
                            <button
                              classList={{ [`${styles.active}`]: viewMode() === 'grid' }}
                              onClick={() => setViewMode('grid')}
                            >
                              Grid
                            </button>
                            <button
                              classList={{ [`${styles.active}`]: viewMode() === 'gallery' }}
                              onClick={() => setViewMode('gallery')}
                            >
                              Gallery
                            </button>
                            <button
                              classList={{ [`${styles.active}`]: viewMode() === 'board' }}
                              onClick={() => setViewMode('board')}
                            >
                              Board
                            </button>
                          </div>
                        </div>
                        <Show when={viewMode() === 'table'}>
                          <DatabaseTable
                            databaseId={selectedNodeId() || ''}
                            columns={nodes().find((n) => n.id === selectedNodeId())?.columns || []}
                            records={records()}
                            onAddRecord={handleAddRecord}
                            onDeleteRecord={handleDeleteRecord}
                            onLoadMore={loadMoreRecords}
                            hasMore={hasMore()}
                          />
                        </Show>
                        <Show when={viewMode() === 'grid'}>
                          <DatabaseGrid
                            records={records()}
                            columns={nodes().find((n) => n.id === selectedNodeId())?.columns || []}
                            onDeleteRecord={handleDeleteRecord}
                          />
                        </Show>
                        <Show when={viewMode() === 'gallery'}>
                          <DatabaseGallery
                            records={records()}
                            columns={nodes().find((n) => n.id === selectedNodeId())?.columns || []}
                            onDeleteRecord={handleDeleteRecord}
                          />
                        </Show>
                        <Show when={viewMode() === 'board'}>
                          <DatabaseBoard
                            records={records()}
                            columns={nodes().find((n) => n.id === selectedNodeId())?.columns || []}
                            onDeleteRecord={handleDeleteRecord}
                          />
                        </Show>
                      </div>
                    </Show>
                  </div>
                </div>
              </Show>
            </Show>
          </main>
        </div>
      </div>
    </Show>
  );
}
