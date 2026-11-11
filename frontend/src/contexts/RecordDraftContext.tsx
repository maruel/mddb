// Retains rejected record edits across workspace routes and owns revision-bound recovery.
import { createContext, createEffect, createSignal, useContext, type ParentComponent } from "solid-js";
import { APIError } from "../useApi";
import { useAuth } from "./AuthContext";

interface Review {
  data: Record<string, unknown> | null;
  revision: string;
  schemaRevision: string;
}

export interface RecordDraft {
  id: number;
  workspaceId: string;
  nodeId: string;
  recordId: string;
  data: Record<string, unknown>;
  review: Review | null;
  busy: boolean;
  error: string | null;
}

interface RecordDraftContextValue {
  drafts: () => RecordDraft[];
  add: (epoch: number, workspaceId: string, nodeId: string, recordId: string, data: Record<string, unknown>) => void;
  review: (id: number) => Promise<void>;
  save: (id: number) => Promise<boolean>;
  dismiss: (id: number) => void;
}

const RecordDraftContext = createContext<RecordDraftContextValue>();

export const RecordDraftProvider: ParentComponent = (props) => {
  const { identity, user, wsApi } = useAuth();
  const [drafts, setDrafts] = createSignal<RecordDraft[]>([]);
  let nextId = 0;
  let epoch = identity();
  const running = new Set<number>();
  createEffect(() => {
    const current = identity();
    if (current !== epoch) {
      epoch = current;
      setDrafts([]);
    }
  });
  function patch(id: number, update: Partial<RecordDraft>) {
    setDrafts((items) => items.map((item) => (item.id === id ? { ...item, ...update } : item)));
  }
  function add(owner: number, workspaceId: string, nodeId: string, recordId: string, data: Record<string, unknown>) {
    if (owner !== identity()) return;
    setDrafts((items) => [
      ...items,
      {
        id: nextId++,
        workspaceId,
        nodeId,
        recordId,
        data: structuredClone(data),
        review: null,
        busy: false,
        error: null,
      },
    ]);
  }
  async function perform(id: number, save: boolean): Promise<boolean> {
    const draft = drafts().find((item) => item.id === id);
    const ws = wsApi();
    if (!draft || !ws || user()?.workspace_id !== draft.workspaceId || running.has(id) || (save && !draft.review))
      return false;
    const owner = identity();
    running.add(id);
    patch(id, { busy: true, error: null });
    try {
      if (save && draft.review) {
        await ws.updateRecord(draft.nodeId, draft.recordId, {
          data: draft.data,
          expected_revision: draft.review.revision,
          expected_schema_revision: draft.review.schemaRevision,
        });
        if (owner !== identity()) return false;
        setDrafts((items) => items.filter((item) => item.id !== id));
        return true;
      }
      const [record, table] = await Promise.all([
        ws.getRecord(draft.nodeId, draft.recordId),
        ws.getTable(draft.nodeId),
      ]);
      if (owner !== identity()) return false;
      patch(id, {
        review: { data: record.data, revision: record.revision, schemaRevision: table.schema_revision },
        busy: false,
      });
    } catch (error) {
      if (owner === identity())
        patch(id, { busy: false, review: null, error: error instanceof APIError ? error.code : "unknown" });
    } finally {
      running.delete(id);
    }
    return false;
  }
  const value: RecordDraftContextValue = {
    drafts,
    add,
    review: async (id) => {
      await perform(id, false);
    },
    save: (id) => perform(id, true),
    dismiss: (id) => {
      if (!running.has(id)) setDrafts((items) => items.filter((item) => item.id !== id));
    },
  };
  return <RecordDraftContext.Provider value={value}>{props.children}</RecordDraftContext.Provider>;
};

export function useRecordDrafts(): RecordDraftContextValue {
  const context = useContext(RecordDraftContext);
  if (!context) throw new Error("RecordDraftProvider is missing");
  return context;
}
