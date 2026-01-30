// Workspace section module wrapping providers and nested routes.

import { type ParentComponent } from 'solid-js';
import { WorkspaceProvider, EditorProvider, RecordsProvider } from '../contexts';

/**
 * WorkspaceSection is the self-contained module for workspace functionality.
 * It wraps children with the necessary providers (WorkspaceProvider, EditorProvider, RecordsProvider).
 *
 * Usage in App.tsx route definitions:
 *   <Route path="/w/:wsId" component={WorkspaceSection}>
 *     <Route path="/" component={WorkspaceLayout}>
 *       <Route path="/" component={WorkspaceRoot} />
 *       <Route path="/:nodeId" component={NodeView} />
 *     </Route>
 *   </Route>
 *
 * Route structure:
 *   /w/:wsId/          -> WorkspaceRoot (redirects to first node)
 *   /w/:wsId/:nodeId   -> NodeView (editor or table)
 */
const WorkspaceSection: ParentComponent = (props) => {
  return (
    <WorkspaceProvider>
      <EditorProvider>
        <RecordsProvider>{props.children}</RecordsProvider>
      </EditorProvider>
    </WorkspaceProvider>
  );
};

export default WorkspaceSection;

// Re-export components for use in route definitions
export { default as WorkspaceLayout } from './WorkspaceLayout';
export { default as WorkspaceRoot } from './WorkspaceRoot';
export { default as NodeView } from './NodeView';
