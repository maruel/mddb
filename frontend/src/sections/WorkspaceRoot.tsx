// Workspace root component that redirects to the first node.

import { createEffect } from 'solid-js';
import { useNavigate, useParams } from '@solidjs/router';
import { useAuth, useWorkspace } from '../contexts';
import { nodeUrl, stripSlug } from '../utils/urls';

/**
 * WorkspaceRoot handles the /w/:wsId/ route.
 * It redirects to the first node in the workspace or shows an empty state.
 */
export default function WorkspaceRoot() {
  const navigate = useNavigate();
  const params = useParams<{ wsId: string }>();
  const { user } = useAuth();
  const { nodes, loading } = useWorkspace();

  // Redirect to first node when nodes are loaded
  createEffect(() => {
    if (loading()) return;

    const nodeList = nodes;
    if (nodeList.length > 0 && nodeList[0]) {
      const firstNode = nodeList[0];
      const u = user();
      const wsId = u?.workspace_id || stripSlug(params.wsId);
      const wsName = u?.workspace_name;
      if (wsId) {
        navigate(nodeUrl(wsId, wsName, firstNode.id, firstNode.title), { replace: true });
      }
    }
  });

  // Return null as we're just handling the redirect
  return null;
}
