import { createSignal, createEffect, For, Show } from 'solid-js';
import MarkdownPreview from './components/MarkdownPreview';
import DatabaseTable from './components/DatabaseTable';
import { debounce } from './utils/debounce';
import styles from './App.module.css';

interface Page {
  id: string;
  title: string;
  created: string;
  modified: string;
}

interface Column {
  id: string;
  name: string;
  type: string;
  options?: string[];
  required?: boolean;
}

interface Database {
  id: string;
  title: string;
  columns: Column[];
  created: string;
  modified: string;
}

interface Record {
  id: string;
  data: Record<string, unknown>;
  created: string;
  modified: string;
}

export default function App() {
  const [pages, setPages] = createSignal<Page[]>([]);
  const [databases, setDatabases] = createSignal<Database[]>([]);
  const [records, setRecords] = createSignal<Record[]>([]);
  const [selectedPageId, setSelectedPageId] = createSignal<string | null>(null);
  const [selectedDatabaseId, setSelectedDatabaseId] = createSignal<string | null>(null);
  const [activeTab, setActiveTab] = createSignal<'pages' | 'databases'>('pages');
  const [title, setTitle] = createSignal('');
  const [content, setContent] = createSignal('');
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [hasUnsavedChanges, setHasUnsavedChanges] = createSignal(false);
  const [autoSaveStatus, setAutoSaveStatus] = createSignal<'idle' | 'saving' | 'saved'>('idle');

  // Debounced auto-save function
  const debouncedAutoSave = debounce(async () => {
    const pageId = selectedPageId();
    if (!pageId || !hasUnsavedChanges()) return;

    try {
      setAutoSaveStatus('saving');
      await fetch(`/api/pages/${pageId}`, {
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

  // Load pages and databases on mount
  createEffect(() => {
    loadPages();
    loadDatabases();
  });

  async function loadPages() {
    try {
      setLoading(true);
      const res = await fetch('/api/pages');
      const data = await res.json();
      setPages(data.pages || []);
      setError(null);
    } catch (err) {
      setError('Failed to load pages: ' + err);
    } finally {
      setLoading(false);
    }
  }

  async function loadPage(id: string) {
    try {
      setLoading(true);
      const res = await fetch(`/api/pages/${id}`);
      const pageData = await res.json();
      setTitle(pageData.title);
      setContent(pageData.content);
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);
    } catch (err) {
      setError('Failed to load page: ' + err);
    } finally {
      setLoading(false);
    }
  }

  async function createPage() {
    if (!title().trim()) {
      setError('Title is required');
      return;
    }

    try {
      setLoading(true);
      await fetch('/api/pages', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: title(), content: content() }),
      });
      await loadPages();
      setTitle('');
      setContent('');
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);
    } catch (err) {
      setError('Failed to create page: ' + err);
    } finally {
      setLoading(false);
    }
  }

  async function savePage() {
    const pageId = selectedPageId();
    if (!pageId) return;

    try {
      setLoading(true);
      await fetch(`/api/pages/${pageId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: title(), content: content() }),
      });
      await loadPages();
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);
    } catch (err) {
      setError('Failed to save page: ' + err);
    } finally {
      setLoading(false);
    }
  }

  async function deleteCurrentPage() {
    const pageId = selectedPageId();
    if (!pageId) return;

    if (!confirm('Are you sure you want to delete this page?')) return;

    try {
      setLoading(true);
      await fetch(`/api/pages/${pageId}`, { method: 'DELETE' });
      await loadPages();
      setSelectedPageId(null);
      setTitle('');
      setContent('');
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);
    } catch (err) {
      setError('Failed to delete page: ' + err);
    } finally {
      setLoading(false);
    }
  }

  const handlePageClick = (page: Page) => {
    setSelectedPageId(page.id);
    loadPage(page.id);
  };

  // Database operations
  async function loadDatabases() {
    try {
      setLoading(true);
      const res = await fetch('/api/databases');
      const data = await res.json();
      setDatabases(data.databases || []);
      setError(null);
    } catch (err) {
      setError('Failed to load databases: ' + err);
    } finally {
      setLoading(false);
    }
  }

  async function loadDatabase(id: string) {
    try {
      setLoading(true);
      const res = await fetch(`/api/databases/${id}`);
      const data = await res.json();
      setTitle(data.title);
      setError(null);

      // Load records
      const recordsRes = await fetch(`/api/databases/${id}/records`);
      const recordsData = await recordsRes.json();
      setRecords(recordsData.records || []);
    } catch (err) {
      setError('Failed to load database: ' + err);
    } finally {
      setLoading(false);
    }
  }

  async function handleAddRecord(data: Record<string, unknown>) {
    const dbId = selectedDatabaseId();
    if (!dbId) return;

    try {
      setLoading(true);
      const res = await fetch(`/api/databases/${dbId}/records`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ data }),
      });

      if (!res.ok) {
        setError('Failed to create record');
        return;
      }

      // Reload records
      await loadDatabase(dbId);
      setError(null);
    } catch (err) {
      setError('Failed to add record: ' + err);
    } finally {
      setLoading(false);
    }
  }

  async function handleDeleteRecord(recordId: string) {
    const dbId = selectedDatabaseId();
    if (!dbId) return;

    if (!confirm('Delete this record?')) return;

    try {
      setLoading(true);
      await fetch(`/api/databases/${dbId}/records/${recordId}`, { method: 'DELETE' });
      await loadDatabase(dbId);
      setError(null);
    } catch (err) {
      setError('Failed to delete record: ' + err);
    } finally {
      setLoading(false);
    }
  }

  const handleDatabaseClick = (db: Database) => {
    setSelectedDatabaseId(db.id);
    setSelectedPageId(null);
    loadDatabase(db.id);
  };

  return (
    <div class={styles.app}>
      <header class={styles.header}>
        <h1>mddb</h1>
        <p>A markdown-based document and database system</p>
      </header>

      <div class={styles.container}>
        <aside class={styles.sidebar}>
          <div class={styles.tabBar}>
            <button
              class={styles.tab}
              classList={{ [styles.active]: activeTab() === 'pages' }}
              onClick={() => {
                setActiveTab('pages');
                setSelectedDatabaseId(null);
              }}
            >
              Pages
            </button>
            <button
              class={styles.tab}
              classList={{ [styles.active]: activeTab() === 'databases' }}
              onClick={() => {
                setActiveTab('databases');
                setSelectedPageId(null);
              }}
            >
              Databases
            </button>
          </div>

          <Show when={activeTab() === 'pages'}>
            <div class={styles.sidebarHeader}>
              <h2>Pages</h2>
              <button onClick={createPage} disabled={loading()}>
                {loading() ? 'Creating...' : 'New Page'}
              </button>
            </div>

            <Show when={loading()} fallback={null}>
              <p class={styles.loading}>Loading...</p>
            </Show>

            <ul class={styles.pageList}>
              <For each={pages()}>
                {(page) => (
                  <li
                    class={styles.pageItem}
                    classList={{ [styles.active]: selectedPageId() === page.id }}
                    onClick={() => handlePageClick(page)}
                  >
                    <div class={styles.pageTitle}>{page.title}</div>
                    <div class={styles.pageDate}>
                      {new Date(page.modified).toLocaleDateString()}
                    </div>
                  </li>
                )}
              </For>
            </ul>
          </Show>

          <Show when={activeTab() === 'databases'}>
            <div class={styles.sidebarHeader}>
              <h2>Databases</h2>
              <button disabled={loading()}>{loading() ? 'Creating...' : 'New DB'}</button>
            </div>

            <Show when={loading()} fallback={null}>
              <p class={styles.loading}>Loading...</p>
            </Show>

            <ul class={styles.pageList}>
              <For each={databases()}>
                {(db) => (
                  <li
                    class={styles.pageItem}
                    classList={{ [styles.active]: selectedDatabaseId() === db.id }}
                    onClick={() => handleDatabaseClick(db)}
                  >
                    <div class={styles.pageTitle}>{db.title}</div>
                    <div class={styles.pageDate}>{new Date(db.modified).toLocaleDateString()}</div>
                  </li>
                )}
              </For>
            </ul>
          </Show>
        </aside>

        <main class={styles.main}>
          <Show when={error()} fallback={null}>
            <div class={styles.error}>{error()}</div>
          </Show>

          <Show when={selectedDatabaseId()}>
            <div class={styles.databaseView}>
              <div class={styles.databaseHeader}>
                <h2>{title()}</h2>
              </div>
              <DatabaseTable
                databaseId={selectedDatabaseId() || ''}
                columns={databases().find((db) => db.id === selectedDatabaseId())?.columns || []}
                records={records()}
                onAddRecord={handleAddRecord}
                onDeleteRecord={handleDeleteRecord}
              />
            </div>
          </Show>

          <Show
            when={!selectedPageId() && !selectedDatabaseId()}
            fallback={
              <div class={styles.editor}>
                <div class={styles.editorHeader}>
                  <input
                    type="text"
                    placeholder="Page title"
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
                    <button onClick={savePage} disabled={loading()}>
                      {loading() ? 'Saving...' : 'Save'}
                    </button>
                    <button onClick={deleteCurrentPage} disabled={loading()}>
                      Delete
                    </button>
                  </div>
                </div>
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
              </div>
            }
          >
            <div class={styles.welcome}>
              <h2>Create Your First Page</h2>
              <div class={styles.createForm}>
                <input
                  type="text"
                  placeholder="Page title"
                  value={title()}
                  onInput={(e) => setTitle(e.target.value)}
                  class={styles.titleInput}
                />
                <textarea
                  value={content()}
                  onInput={(e) => setContent(e.target.value)}
                  placeholder="Write your content in markdown..."
                  class={styles.contentInput}
                />
                <button onClick={createPage} disabled={loading()} class={styles.createButton}>
                  {loading() ? 'Creating...' : 'Create Page'}
                </button>
              </div>
            </div>
          </Show>
        </main>
      </div>
    </div>
  );
}
