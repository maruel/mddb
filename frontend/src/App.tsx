import { createSignal, createEffect, createMemo, For, Show, onMount, onCleanup } from 'solid-js';
import SidebarNode from './components/SidebarNode';
import MarkdownPreview from './components/MarkdownPreview';
import TableTable from './components/TableTable';
import TableGrid from './components/TableGrid';
import TableGallery from './components/TableGallery';
import TableBoard from './components/TableBoard';
import WorkspaceSettings from './components/WorkspaceSettings';
import Onboarding from './components/Onboarding';
import Auth from './components/Auth';
import Privacy from './components/Privacy';
import Terms from './components/Terms';
import PWAInstallBanner from './components/PWAInstallBanner';
import CreateOrgModal from './components/CreateOrgModal';
import UserMenu from './components/UserMenu';
import { debounce } from './utils/debounce';
import { useI18n, type Locale } from './i18n';
import { createApi, APIError } from './useApi';
import type { NodeResponse, DataRecordResponse, Commit, UserResponse } from './types.gen';
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
  const { t, locale, setLocale } = useI18n();
  const [user, setUser] = createSignal<UserResponse | null>(null);
  const [token, setToken] = createSignal<string | null>(localStorage.getItem('mddb_token'));
  const [nodes, setNodes] = createSignal<NodeResponse[]>([]);
  const [records, setRecords] = createSignal<DataRecordResponse[]>([]);
  const [selectedNodeId, setSelectedNodeId] = createSignal<string | null>(null);
  const [showSettings, setShowSettings] = createSignal(false);
  const [isPrivacyPage, setIsPrivacyPage] = createSignal(false);
  const [isTermsPage, setIsTermsPage] = createSignal(false);
  const [viewMode, setViewMode] = createSignal<'table' | 'grid' | 'gallery' | 'board'>('table');
  const [title, setTitle] = createSignal('');
  const [content, setContent] = createSignal('');
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [hasUnsavedChanges, setHasUnsavedChanges] = createSignal(false);
  const [autoSaveStatus, setAutoSaveStatus] = createSignal<'idle' | 'saving' | 'saved'>('idle');
  const [showCreateOrg, setShowCreateOrg] = createSignal(false);
  const [showGitSetup, setShowGitSetup] = createSignal(false);

  // History state
  const [showHistory, setShowHistory] = createSignal(false);
  const [history, setHistory] = createSignal<Commit[]>([]);

  // Pagination
  const [hasMore, setHasMore] = createSignal(false);
  const PAGE_SIZE = 50;

  const logout = async () => {
    // Call logout API to revoke the session server-side
    const currentToken = token();
    if (currentToken) {
      try {
        const logoutApi = createApi(
          () => currentToken,
          () => {}
        );
        await logoutApi.auth.logout();
      } catch {
        // Ignore errors - proceed with local logout even if server call fails
      }
    }
    localStorage.removeItem('mddb_token');
    setToken(null);
    setUser(null);
    window.history.pushState(null, '', '/');
  };

  // Create API client with auth
  const api = createMemo(() => createApi(() => token(), logout));

  // Get org-scoped API client
  const orgApi = createMemo(() => {
    const orgID = user()?.organization_id;
    return orgID ? api().org(orgID) : null;
  });

  const handleLogin = (newToken: string, userData: UserResponse) => {
    localStorage.setItem('mddb_token', newToken);
    setToken(newToken);
    setUser(userData);
  };

  async function switchOrg(orgId: string, redirect = true) {
    try {
      setLoading(true);
      const data = await api().auth.switchOrg({ org_id: orgId });
      if (!data.user) {
        throw new Error('No user data returned');
      }
      handleLogin(data.token, data.user);
      setSelectedNodeId(null);
      if (redirect) {
        window.history.pushState(null, '', '/');
      }
      await loadNodes();
    } catch (err) {
      if (err instanceof APIError) {
        setError(`${t('errors.failedToSwitch')}: ${err.message}`);
      } else {
        setError(`${t('errors.failedToSwitch')}: ${err}`);
      }
      throw err;
    } finally {
      setLoading(false);
    }
  }

  async function createOrganization(data: { name: string; welcomePageTitle: string; welcomePageContent: string }) {
    const org = await api().organizations.create({
      name: data.name,
      welcome_page_title: data.welcomePageTitle,
      welcome_page_content: data.welcomePageContent,
    });
    // Refresh user data and switch to the new org
    await switchOrg(org.id);
    // Show git setup prompt after org creation
    setShowGitSetup(true);
  }

  // Debounced auto-save function
  const debouncedAutoSave = debounce(async () => {
    const nodeId = selectedNodeId();
    const org = orgApi();
    if (!nodeId || !hasUnsavedChanges() || !org) return;

    try {
      setAutoSaveStatus('saving');
      await org.pages.update(nodeId, { title: title(), content: content() });
      setHasUnsavedChanges(false);
      setAutoSaveStatus('saved');

      // Update URL if title changed
      const slug = slugify(title());
      const orgId = user()?.organization_id;
      if (orgId) {
        const currentPath = window.location.pathname;
        const newPath = `/${orgId}/${nodeId}${slug ? '+' + slug : ''}`;
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
      setError(`${t('errors.autoSaveFailed')}: ${err}`);
      setAutoSaveStatus('idle');
    }
  }, 2000);

  // Sync locale with user settings
  createEffect(() => {
    const u = user();
    const userLang = u?.settings?.language as Locale | undefined;
    if (userLang && ['en', 'fr', 'de', 'es'].includes(userLang) && userLang !== locale()) {
      setLocale(userLang);
      localStorage.setItem('mddb_locale', userLang);
    }
  });

  // Handle OAuth token from URL on mount (runs once)
  onMount(() => {
    const urlParams = new URLSearchParams(window.location.search);
    const urlToken = urlParams.get('token');
    if (urlToken) {
      localStorage.setItem('mddb_token', urlToken);
      setToken(urlToken);
      // Clean up URL query params but keep pathname
      window.history.replaceState({}, document.title, window.location.pathname);
    }
  });

  // Fetch user when we have a token but no user data
  let fetchingUser = false;
  createEffect(() => {
    const tok = token();
    const u = user();
    if (tok && !u && !fetchingUser) {
      fetchingUser = true;
      (async () => {
        try {
          const data = await api().auth.me.get();
          setUser(data);
        } catch (err) {
          console.error('Failed to load user', err);
          // Clear invalid token on auth failure
          if (err instanceof APIError && err.status === 401) {
            localStorage.removeItem('mddb_token');
            setToken(null);
          }
        } finally {
          fetchingUser = false;
        }
      })();
    }
  });

  // Handle browser back/forward and initial URL
  const handlePopState = async () => {
    const path = window.location.pathname;

    if (path === '/privacy') {
      setIsPrivacyPage(true);
      setIsTermsPage(false);
      return;
    }
    if (path === '/terms') {
      setIsTermsPage(true);
      setIsPrivacyPage(false);
      return;
    }
    setIsPrivacyPage(false);
    setIsTermsPage(false);

    // Check for /orgID/nodeID (slug separator is +)
    const matchWithOrg = path.match(/^\/([^/]+)\/([a-zA-Z0-9_-]+)(?:\+.*)?$/);
    if (matchWithOrg && matchWithOrg[1] && matchWithOrg[2]) {
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
            setError(t('errors.noAccessToOrg') || 'You do not have access to this organization');
            return;
          }
        } catch {
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
    } else {
      // If not logged in, we still need to check for /privacy
      handlePopState();
    }
  });

  async function loadNodes() {
    const org = orgApi();
    if (!org) return;

    try {
      setLoading(true);
      const data = await org.nodes.list();
      setNodes((data.nodes?.filter(Boolean) as NodeResponse[]) || []);
      setError(null);
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function loadNode(id: string, pushState = true) {
    const org = orgApi();
    if (!org) return;

    try {
      setLoading(true);
      setShowHistory(false);
      const nodeData = await org.nodes.get(id);

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
        const url = `/${orgId}/${nodeData.id}${slug ? '+' + slug : ''}`;
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

      // If it's a table or hybrid, load records
      if (nodeData.type === 'table' || nodeData.type === 'hybrid') {
        const recordsData = await org.tables.records.list(id, { Offset: 0, Limit: PAGE_SIZE });
        const loadedRecords = (recordsData.records || []) as DataRecordResponse[];
        setRecords(loadedRecords);
        setHasMore(loadedRecords.length === PAGE_SIZE);
      } else {
        setRecords([]);
        setHasMore(false);
      }
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function loadHistory(nodeId: string) {
    if (showHistory()) {
      setShowHistory(false);
      return;
    }

    const org = orgApi();
    if (!org) return;

    try {
      setLoading(true);
      const data = await org.pages.history.list(nodeId, { Limit: 100 });
      setHistory((data.history?.filter(Boolean) as Commit[]) || []);
      setShowHistory(true);
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function loadVersion(nodeId: string, hash: string) {
    if (!confirm(t('editor.restoreConfirm') || 'This will replace current editor content. Continue?')) return;

    const org = orgApi();
    if (!org) return;

    try {
      setLoading(true);
      const data = await org.pages.history.get(nodeId, hash);
      setContent(data.content || '');
      setHasUnsavedChanges(true); // Mark as modified
      setShowHistory(false);
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function createNode(type: 'document' | 'table' = 'document') {
    if (!title().trim()) {
      setError(t('errors.titleRequired') || 'Title is required');
      return;
    }

    const org = orgApi();
    if (!org) return;

    try {
      setLoading(true);
      const newNode = await org.nodes.create({ title: title(), type });
      await loadNodes();
      loadNode(newNode.id);
      setTitle('');
      setContent('');
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);
    } catch (err) {
      setError(`${t('errors.failedToCreate')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function saveNode() {
    const nodeId = selectedNodeId();
    const org = orgApi();
    if (!nodeId || !org) return;

    try {
      setLoading(true);
      await org.pages.update(nodeId, { title: title(), content: content() });
      await loadNodes();
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);

      // Update URL if title changed
      const slug = slugify(title());
      const currentPath = window.location.pathname;
      const orgId = user()?.organization_id;
      if (orgId) {
        const newPath = `/${orgId}/${nodeId}${slug ? '+' + slug : ''}`;
        if (currentPath !== newPath) {
          window.history.replaceState(null, '', newPath);
        }
      }
    } catch (err) {
      setError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function deleteCurrentNode() {
    const nodeId = selectedNodeId();
    const org = orgApi();
    if (!nodeId || !org) return;

    if (!confirm(t('table.confirmDeleteRecord') || 'Delete this record?')) return;

    try {
      setLoading(true);
      await org.pages.delete(nodeId);
      await loadNodes();
      setSelectedNodeId(null);
      setTitle('');
      setContent('');
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);
      window.history.pushState(null, '', '/');
    } catch (err) {
      setError(`${t('errors.failedToDelete')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  const handleNodeClick = (node: NodeResponse) => {
    loadNode(node.id);
  };

  // Recursively find a node by ID in the tree
  const findNodeById = (nodeId: string | null): NodeResponse | undefined => {
    if (!nodeId) return undefined;

    const search = (nodeList: NodeResponse[]): NodeResponse | undefined => {
      for (const node of nodeList) {
        if (node.id === nodeId) return node;
        if (node.children) {
          const found = search(node.children.filter((c): c is NodeResponse => !!c));
          if (found) return found;
        }
      }
      return undefined;
    };

    return search(nodes());
  };

  const getBreadcrumbs = (nodeId: string | null): NodeResponse[] => {
    if (!nodeId) return [];
    const path: NodeResponse[] = [];

    const findPath = (currentNodes: NodeResponse[], targetId: string): boolean => {
      for (const node of currentNodes) {
        if (node.id === targetId) {
          path.push(node);
          return true;
        }
        if (
          node.children &&
          findPath(
            node.children.filter((c): c is NodeResponse => !!c),
            targetId
          )
        ) {
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
    const org = orgApi();
    if (!nodeId || !org) return;

    try {
      setLoading(true);
      await org.tables.records.create(nodeId, { data });
      // Reload records
      loadNode(nodeId);
      setError(null);
    } catch (err) {
      setError(`${t('errors.failedToCreate')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function handleDeleteRecord(recordId: string) {
    const nodeId = selectedNodeId();
    const org = orgApi();
    if (!nodeId || !org) return;

    if (!confirm(t('table.confirmDeleteRecord') || 'Delete this record?')) return;

    try {
      setLoading(true);
      await org.tables.records.delete(nodeId, recordId);
      loadNode(nodeId);
      setError(null);
    } catch (err) {
      setError(`${t('errors.failedToDelete')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function loadMoreRecords() {
    const nodeId = selectedNodeId();
    const org = orgApi();
    if (!nodeId || loading() || !org) return;

    try {
      setLoading(true);
      const offset = records().length;
      const data = await org.tables.records.list(nodeId, { Offset: offset, Limit: PAGE_SIZE });
      const newRecords = (data.records || []) as DataRecordResponse[];
      setRecords([...records(), ...newRecords]);
      setHasMore(newRecords.length === PAGE_SIZE);
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  return (
    <>
      <Show when={!isPrivacyPage()} fallback={<Privacy />}>
        <Show when={!isTermsPage()} fallback={<Terms />}>
          <Show when={user()} fallback={<Auth onLogin={handleLogin} />}>
            <>
              {/* First org creation for new users with no memberships */}
              <Show when={(user()?.memberships?.length ?? 0) === 0}>
                <CreateOrgModal isFirstOrg={true} onClose={() => {}} onCreate={createOrganization} />
              </Show>
              <Show when={showGitSetup()}>
                <Onboarding
                  user={user() as UserResponse}
                  token={token() as string}
                  onComplete={() => setShowGitSetup(false)}
                />
              </Show>
              <div class={styles.app}>
                <header class={styles.header}>
                  <div class={styles.headerTitle}>
                    <h1>{t('app.title')}</h1>
                    <p>{t('app.tagline')}</p>
                  </div>
                  <div class={styles.userInfo}>
                    <Show when={(user()?.memberships?.length ?? 0) > 1}>
                      <select
                        class={styles.orgSwitcher}
                        value={user()?.organization_id}
                        onChange={(e) => switchOrg(e.target.value)}
                      >
                        <For each={user()?.memberships}>
                          {(m) => <option value={m.organization_id}>{m.organization_name || m.organization_id}</option>}
                        </For>
                      </select>
                    </Show>
                    <Show when={user()?.memberships?.length === 1}>
                      <span class={styles.orgName}>
                        {user()?.memberships?.[0]?.organization_name || t('app.myOrg')}
                      </span>
                    </Show>
                    <button
                      class={styles.createOrgButton}
                      onClick={() => setShowCreateOrg(true)}
                      title={t('createOrg.title') || 'Create Organization'}
                    >
                      +
                    </button>
                    <UserMenu user={user() as UserResponse} onLogout={logout} />
                  </div>
                </header>

                <div class={styles.container}>
                  <aside class={styles.sidebar}>
                    <div class={styles.sidebarHeader}>
                      <h2>{t('app.workspace')}</h2>
                      <div class={styles.sidebarActions}>
                        <button
                          onClick={() => {
                            setShowSettings(true);
                            setSelectedNodeId(null);
                          }}
                          title={t('app.settings') || 'Workspace Settings'}
                        >
                          ⚙
                        </button>
                        <button
                          onClick={() => {
                            setShowSettings(false);
                            createNode('document');
                          }}
                          title={t('app.newPage') || 'New Page'}
                        >
                          +P
                        </button>
                        <button
                          onClick={() => {
                            setShowSettings(false);
                            createNode('table');
                          }}
                          title={t('app.newTable') || 'New Table'}
                        >
                          +D
                        </button>
                      </div>
                    </div>

                    <Show when={loading() && nodes().length === 0} fallback={null}>
                      <p class={styles.loading}>{t('common.loading')}</p>
                    </Show>

                    <ul class={styles.pageList}>
                      <For each={nodes()}>
                        {(node) => (
                          <SidebarNode node={node} selectedId={selectedNodeId()} onSelect={handleNodeClick} depth={0} />
                        )}
                      </For>
                    </ul>

                    <div class={styles.sidebarFooter}>
                      <a
                        href="/privacy"
                        onClick={(e) => {
                          e.preventDefault();
                          window.history.pushState(null, '', '/privacy');
                          window.dispatchEvent(new PopStateEvent('popstate'));
                        }}
                      >
                        {t('app.privacyPolicy')}
                      </a>
                      <span style={{ margin: '0 0.5rem', color: '#ccc' }}>|</span>
                      <a
                        href="/terms"
                        onClick={(e) => {
                          e.preventDefault();
                          window.history.pushState(null, '', '/terms');
                          window.dispatchEvent(new PopStateEvent('popstate'));
                        }}
                      >
                        {t('app.terms')}
                      </a>
                    </div>
                  </aside>

                  <main class={styles.main}>
                    <Show when={showSettings() && user() && token()}>
                      <WorkspaceSettings
                        user={user() as UserResponse}
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
                            <h2>{t('welcome.title')}</h2>
                            <p>{t('welcome.subtitle')}</p>
                            <div class={styles.createForm}>
                              <input
                                type="text"
                                placeholder={t('editor.titlePlaceholder') || 'Title'}
                                value={title()}
                                onInput={(e) => setTitle(e.target.value)}
                                class={styles.titleInput}
                              />
                              <div class={styles.welcomeActions}>
                                <button onClick={() => createNode('document')} class={styles.createButton}>
                                  {t('welcome.createPage')}
                                </button>
                                <button onClick={() => createNode('table')} class={styles.createButton}>
                                  {t('welcome.createTable')}
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
                              placeholder={t('editor.titlePlaceholder') || 'Title'}
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
                                <span class={styles.unsavedIndicator}>● {t('editor.unsaved')}</span>
                              </Show>
                              <Show when={autoSaveStatus() === 'saving'}>
                                <span class={styles.savingIndicator}>⟳ {t('common.saving')}</span>
                              </Show>
                              <Show when={autoSaveStatus() === 'saved'}>
                                <span class={styles.savedIndicator}>✓ {t('common.saved')}</span>
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
                                {showHistory() ? t('editor.hideHistory') : t('editor.history')}
                              </button>
                              <button onClick={saveNode} disabled={loading()}>
                                {loading() ? t('common.saving') : t('common.save')}
                              </button>
                              <button onClick={deleteCurrentNode} disabled={loading()}>
                                {t('common.delete')}
                              </button>
                            </div>
                          </div>

                          <Show when={showHistory()}>
                            <div class={styles.historyPanel}>
                              <h3>{t('editor.versionHistory')}</h3>
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
                                        <span class={styles.historyHash}>{commit.hash.substring(0, 7)}</span>
                                      </div>
                                      <div class={styles.historyMessage}>{commit.message}</div>
                                    </li>
                                  )}
                                </For>
                                <Show when={history().length === 0}>
                                  <li class={styles.historyItem}>{t('editor.noHistory')}</li>
                                </Show>
                              </ul>
                            </div>
                          </Show>

                          <div class={styles.nodeContent}>
                            {/* Always show markdown content if it exists or if node is document/hybrid */}
                            <Show when={findNodeById(selectedNodeId())?.type !== 'table'}>
                              <div class={styles.editorContent}>
                                <textarea
                                  value={content()}
                                  onInput={(e) => {
                                    setContent(e.target.value);
                                    setHasUnsavedChanges(true);
                                    debouncedAutoSave();
                                  }}
                                  placeholder={t('editor.contentPlaceholder') || 'Write your content in markdown...'}
                                  class={styles.contentInput}
                                />
                                <MarkdownPreview content={content()} orgId={user()?.organization_id} />
                              </div>
                            </Show>

                            {/* Show table if node is table or hybrid */}
                            <Show when={findNodeById(selectedNodeId())?.type !== 'document'}>
                              <div class={styles.tableView}>
                                <div class={styles.tableHeader}>
                                  <h3>{t('table.records')}</h3>
                                  <div class={styles.viewToggle}>
                                    <button
                                      classList={{ [`${styles.active}`]: viewMode() === 'table' }}
                                      onClick={() => setViewMode('table')}
                                    >
                                      {t('table.table')}
                                    </button>
                                    <button
                                      classList={{ [`${styles.active}`]: viewMode() === 'grid' }}
                                      onClick={() => setViewMode('grid')}
                                    >
                                      {t('table.grid')}
                                    </button>
                                    <button
                                      classList={{ [`${styles.active}`]: viewMode() === 'gallery' }}
                                      onClick={() => setViewMode('gallery')}
                                    >
                                      {t('table.gallery')}
                                    </button>
                                    <button
                                      classList={{ [`${styles.active}`]: viewMode() === 'board' }}
                                      onClick={() => setViewMode('board')}
                                    >
                                      {t('table.board')}
                                    </button>
                                  </div>
                                </div>
                                <Show when={viewMode() === 'table'}>
                                  <TableTable
                                    tableId={selectedNodeId() || ''}
                                    columns={findNodeById(selectedNodeId())?.properties || []}
                                    records={records()}
                                    onAddRecord={handleAddRecord}
                                    onDeleteRecord={handleDeleteRecord}
                                    onLoadMore={loadMoreRecords}
                                    hasMore={hasMore()}
                                  />
                                </Show>
                                <Show when={viewMode() === 'grid'}>
                                  <TableGrid
                                    records={records()}
                                    columns={findNodeById(selectedNodeId())?.properties || []}
                                    onDeleteRecord={handleDeleteRecord}
                                  />
                                </Show>
                                <Show when={viewMode() === 'gallery'}>
                                  <TableGallery
                                    records={records()}
                                    columns={findNodeById(selectedNodeId())?.properties || []}
                                    onDeleteRecord={handleDeleteRecord}
                                  />
                                </Show>
                                <Show when={viewMode() === 'board'}>
                                  <TableBoard
                                    records={records()}
                                    columns={findNodeById(selectedNodeId())?.properties || []}
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
            </>
          </Show>
        </Show>
      </Show>
      <PWAInstallBanner />
      <Show when={showCreateOrg()}>
        <CreateOrgModal onClose={() => setShowCreateOrg(false)} onCreate={createOrganization} />
      </Show>
    </>
  );
}
