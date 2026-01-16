import { createSignal, createEffect, For, Show } from 'solid-js';
import styles from './App.module.css';

interface Page {
  id: string;
  title: string;
  created: string;
  modified: string;
}

interface PageDetail {
  id: string;
  title: string;
  content: string;
}

export default function App() {
  const [pages, setPages] = createSignal<Page[]>([]);
  const [selectedPageId, setSelectedPageId] = createSignal<string | null>(null);
  const [selectedPage, setSelectedPage] = createSignal<PageDetail | null>(null);
  const [title, setTitle] = createSignal('');
  const [content, setContent] = createSignal('');
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);

  // Load pages on mount
  createEffect(() => {
    loadPages();
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
      const data = await res.json();
      setSelectedPage(data);
      setTitle(data.title);
      setContent(data.content);
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
      const res = await fetch('/api/pages', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: title(), content: content() }),
      });
      const data = await res.json();
      await loadPages();
      setTitle('');
      setContent('');
      setError(null);
    } catch (err) {
      setError('Failed to create page: ' + err);
    } finally {
      setLoading(false);
    }
  }

  async function updatePage() {
    const pageId = selectedPageId();
    if (!pageId) return;

    try {
      setLoading(true);
      const res = await fetch(`/api/pages/${pageId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: title(), content: content() }),
      });
      await loadPages();
      await loadPage(pageId);
      setError(null);
    } catch (err) {
      setError('Failed to update page: ' + err);
    } finally {
      setLoading(false);
    }
  }

  async function deletePage() {
    const pageId = selectedPageId();
    if (!pageId) return;

    if (!confirm('Are you sure you want to delete this page?')) return;

    try {
      setLoading(true);
      await fetch(`/api/pages/${pageId}`, { method: 'DELETE' });
      await loadPages();
      setSelectedPageId(null);
      setSelectedPage(null);
      setTitle('');
      setContent('');
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

  return (
    <div class={styles.app}>
      <header class={styles.header}>
        <h1>mddb</h1>
        <p>A markdown-based document and database system</p>
      </header>

      <div class={styles.container}>
        <aside class={styles.sidebar}>
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
        </aside>

        <main class={styles.main}>
          <Show when={error()} fallback={null}>
            <div class={styles.error}>{error()}</div>
          </Show>

          <Show
            when={!selectedPageId()}
            fallback={
              <div class={styles.editor}>
                <div class={styles.editorHeader}>
                  <input
                    type="text"
                    placeholder="Page title"
                    value={title()}
                    onInput={(e) => setTitle(e.target.value)}
                    class={styles.titleInput}
                  />
                  <div class={styles.editorActions}>
                    <button onClick={updatePage} disabled={loading()}>
                      {loading() ? 'Saving...' : 'Save'}
                    </button>
                    <button onClick={deletePage} disabled={loading()}>
                      Delete
                    </button>
                  </div>
                </div>
                <textarea
                  value={content()}
                  onInput={(e) => setContent(e.target.value)}
                  placeholder="Write your content in markdown..."
                  class={styles.contentInput}
                />
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
