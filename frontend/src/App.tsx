import { createSignal, createEffect, For, Show } from 'solid-js';
import MarkdownPreview from './components/MarkdownPreview';
import DatabaseTable from './components/DatabaseTable';
import { debounce } from './utils/debounce';
import styles from './App.module.css';

interface Node {
  id: string;
  parent_id?: string;
  title: string;
  content?: string;
  columns?: Column[];
  created: string;
  modified: string;
  type: 'document' | 'database' | 'hybrid';
}

interface Column {
  id: string;
  name: string;
  type: string;
  options?: string[];
  required?: boolean;
}

interface Record {
  id: string;
  data: Record<string, unknown>;
  created: string;
  modified: string;
}

interface Commit {
  hash: string;
  message: string;
  timestamp: string;
}

export default function App() {
  const [nodes, setNodes] = createSignal<Node[]>([]);
  const [records, setRecords] = createSignal<Record[]>([]);
  const [selectedNodeId, setSelectedNodeId] = createSignal<string | null>(null);
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

  // Debounced auto-save function
  const debouncedAutoSave = debounce(async () => {
    const nodeId = selectedNodeId();
    if (!nodeId || !hasUnsavedChanges()) return;

    try {
      setAutoSaveStatus('saving');
      await fetch(`/api/pages/${nodeId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: title(), content: content() }),
      });
      setHasUnsavedChanges(false);
      setAutoSaveStatus('saved');
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

  // Load nodes on mount
  createEffect(() => {
    loadNodes();
  });

  async function loadNodes() {
    try {
      setLoading(true);
      const res = await fetch('/api/nodes');
      const data = await res.json();
      setNodes(data.nodes || []);
      setError(null);
    } catch (err) {
      setError('Failed to load nodes: ' + err);
    } finally {
      setLoading(false);
    }
  }

  async function loadNode(id: string) {
    try {
      setLoading(true);
      setShowHistory(false);
      const res = await fetch(`/api/nodes/${id}`);
      const nodeData = await res.json();

      setSelectedNodeId(nodeData.id);
      setTitle(nodeData.title);
      setContent(nodeData.content || '');
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);

      // If it's a database or hybrid, load records
      if (nodeData.type === 'database' || nodeData.type === 'hybrid') {
        const recordsRes = await fetch(`/api/databases/${id}/records?offset=0&limit=${PAGE_SIZE}`);
        const recordsData = await recordsRes.json();
        const loadedRecords = recordsData.records || [];
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
      const res = await fetch(`/api/pages/${nodeId}/history`);
      const data = await res.json();
      setHistory(data.history || []);
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
      const res = await fetch(`/api/pages/${nodeId}/history/${hash}`);
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
      const res = await fetch('/api/nodes', {
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
      await fetch(`/api/pages/${nodeId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: title(), content: content() }),
      });
      await loadNodes();
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);
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
      await fetch(`/api/pages/${nodeId}`, { method: 'DELETE' });
      await loadNodes();
      setSelectedNodeId(null);
      setTitle('');
      setContent('');
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);
    } catch (err) {
      setError('Failed to delete node: ' + err);
    } finally {
      setLoading(false);
    }
  }

  const handleNodeClick = (node: Node) => {
    loadNode(node.id);
  };

  async function handleAddRecord(data: Record<string, unknown>) {
    const nodeId = selectedNodeId();
    if (!nodeId) return;

    try {
      setLoading(true);
      const res = await fetch(`/api/databases/${nodeId}/records`, {
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
      await fetch(`/api/databases/${nodeId}/records/${recordId}`, { method: 'DELETE' });
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
      const res = await fetch(
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
    <div class={styles.app}>
      <header class={styles.header}>
        <h1>mddb</h1>
        <p>A seamless markdown-based document and database system</p>
      </header>

      <div class={styles.container}>
        <aside class={styles.sidebar}>
          <div class={styles.sidebarHeader}>
            <h2>Workspace</h2>
            <div class={styles.sidebarActions}>
              <button onClick={() => createNode('document')} title="New Page">
                +P
              </button>
              <button onClick={() => createNode('database')} title="New Database">
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
                <li
                  class={styles.pageItem}
                  classList={{ [styles.active]: selectedNodeId() === node.id }}
                  onClick={() => handleNodeClick(node)}
                >
                  <div class={styles.pageTitle}>
                    <span class={styles.nodeIcon}>{node.type === 'database' ? '📊' : '📄'}</span>
                    {node.title}
                  </div>
                  <div class={styles.pageDate}>{new Date(node.modified).toLocaleDateString()}</div>
                </li>
              )}
            </For>
          </ul>
        </aside>

        <main class={styles.main}>
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
                  <button onClick={() => loadHistory(selectedNodeId()!)} disabled={loading()}>
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
                          onClick={() => loadVersion(selectedNodeId()!, commit.hash)}
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
                      <li class={styles.historyItem}>No history available</li>
                    </Show>
                  </ul>
                </div>
              </Show>

              <div class={styles.nodeContent}>
                {/* Always show markdown content if it exists or if node is document/hybrid */}
                <Show when={nodes().find((n) => n.id === selectedNodeId())?.type !== 'database'}>
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
                    <MarkdownPreview content={content()} />
                  </div>
                </Show>

                {/* Show database table if node is database or hybrid */}
                <Show when={nodes().find((n) => n.id === selectedNodeId())?.type !== 'document'}>
                  <div class={styles.databaseView}>
                    <div class={styles.databaseHeader}>
                      <h3>Database Records</h3>
                    </div>
                    <DatabaseTable
                      databaseId={selectedNodeId() || ''}
                      columns={nodes().find((n) => n.id === selectedNodeId())?.columns || []}
                      records={records()}
                      onAddRecord={handleAddRecord}
                      onDeleteRecord={handleDeleteRecord}
                      onLoadMore={loadMoreRecords}
                      hasMore={hasMore()}
                    />
                  </div>
                </Show>
              </div>
            </div>
          </Show>
        </main>
      </div>
    </div>
  );
}
