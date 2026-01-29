// Editor context providing title, content, auto-save, and history management.

import {
  createContext,
  useContext,
  createSignal,
  createEffect,
  onCleanup,
  type ParentComponent,
  type Accessor,
} from 'solid-js';
import { useAuth } from './AuthContext';
import { useWorkspace } from './WorkspaceContext';
import { useI18n } from '../i18n';
import { debounce } from '../utils/debounce';
import { nodeUrl } from '../utils/urls';
import type { Commit } from '@sdk/types.gen';

/** Map of asset filename to signed URL */
export type AssetUrlMap = Record<string, string>;

interface EditorContextValue {
  // Editor state
  title: Accessor<string>;
  setTitle: (title: string) => void;
  content: Accessor<string>;
  setContent: (content: string) => void;
  hasUnsavedChanges: Accessor<boolean>;
  setHasUnsavedChanges: (has: boolean) => void;
  autoSaveStatus: Accessor<'idle' | 'saving' | 'saved'>;

  // Asset URLs (filename -> signed URL)
  assetUrls: Accessor<AssetUrlMap>;

  // History
  showHistory: Accessor<boolean>;
  setShowHistory: (show: boolean) => void;
  history: Accessor<Commit[]>;
  loadHistory: (nodeId: string) => Promise<void>;
  loadVersion: (nodeId: string, hash: string) => Promise<void>;

  // Actions
  triggerAutoSave: () => void;
  flushAutoSave: () => void;
  resetEditor: () => void;
  handleTitleChange: (newTitle: string) => void;
  handleContentChange: (newContent: string) => void;
}

const EditorContext = createContext<EditorContextValue>();

export const EditorProvider: ParentComponent = (props) => {
  const { t } = useI18n();
  const { user, wsApi } = useAuth();
  const { selectedNodeId, selectedNodeData, updateNodeTitle, setLoading, setError } = useWorkspace();

  // Editor state
  const [title, setTitle] = createSignal('');
  const [content, setContent] = createSignal('');
  const [hasUnsavedChanges, setHasUnsavedChanges] = createSignal(false);
  const [autoSaveStatus, setAutoSaveStatus] = createSignal<'idle' | 'saving' | 'saved'>('idle');

  // Asset URLs accessor - derived from selectedNodeData
  const assetUrls = (): AssetUrlMap => selectedNodeData()?.asset_urls || {};

  // History state
  const [showHistory, setShowHistory] = createSignal(false);
  const [history, setHistory] = createSignal<Commit[]>([]);

  // Debounced auto-save
  const debouncedAutoSave = debounce(async () => {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !hasUnsavedChanges() || !ws) return;

    try {
      setAutoSaveStatus('saving');
      await ws.nodes.page.updatePage(nodeId, { title: title(), content: content() });
      setHasUnsavedChanges(false);
      setAutoSaveStatus('saved');

      // Update URL if title changed
      const wsId = user()?.workspace_id;
      const wsName = user()?.workspace_name;
      if (wsId) {
        const currentPath = window.location.pathname;
        const newPath = nodeUrl(wsId, wsName, nodeId, title());
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

  // Cleanup debounce on unmount
  onCleanup(() => {
    debouncedAutoSave.flush();
  });

  // Sync editor state when selected node changes
  createEffect(() => {
    const node = selectedNodeData();
    if (node) {
      setTitle(node.title);
      setContent(node.content || '');
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setShowHistory(false);
    } else {
      resetEditor();
    }
  });

  function resetEditor() {
    setTitle('');
    setContent('');
    setHasUnsavedChanges(false);
    setAutoSaveStatus('idle');
    setShowHistory(false);
    setHistory([]);
  }

  function handleTitleChange(newTitle: string) {
    setTitle(newTitle);
    const id = selectedNodeId();
    if (id) {
      updateNodeTitle(id, newTitle);
    }
    setHasUnsavedChanges(true);
    debouncedAutoSave();
  }

  function handleContentChange(newContent: string) {
    setContent(newContent);
    setHasUnsavedChanges(true);
    debouncedAutoSave();
  }

  async function loadHistory(nodeId: string) {
    if (showHistory()) {
      setShowHistory(false);
      return;
    }
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      const data = await ws.nodes.history.listNodeVersions(nodeId, { Limit: 100 });
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
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      const data = await ws.nodes.history.getNodeVersion(nodeId, hash);
      setContent(data.content || '');
      setHasUnsavedChanges(true);
      setShowHistory(false);
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  const value: EditorContextValue = {
    title,
    setTitle,
    content,
    setContent,
    hasUnsavedChanges,
    setHasUnsavedChanges,
    autoSaveStatus,
    assetUrls,
    showHistory,
    setShowHistory,
    history,
    loadHistory,
    loadVersion,
    triggerAutoSave: debouncedAutoSave,
    flushAutoSave: debouncedAutoSave.flush,
    resetEditor,
    handleTitleChange,
    handleContentChange,
  };

  return <EditorContext.Provider value={value}>{props.children}</EditorContext.Provider>;
};

export function useEditor(): EditorContextValue {
  const context = useContext(EditorContext);
  if (!context) {
    throw new Error('useEditor must be used within an EditorProvider');
  }
  return context;
}
