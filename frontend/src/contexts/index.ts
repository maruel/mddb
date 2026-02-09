// Context providers for global application state.

export { AuthProvider, useAuth } from './AuthContext';
export { WorkspaceProvider, useWorkspace } from './WorkspaceContext';
export { EditorProvider, useEditor } from './EditorContext';
export { RecordsProvider, useRecords } from './RecordsContext';
export { NotificationProvider, useNotifications } from './NotificationContext';

// Re-export slugify from urls for backward compatibility (but prefer importing from urls directly)
export { slugify } from '../utils/urls';
