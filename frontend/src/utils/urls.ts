// URL construction utilities for consistent routing.

/**
 * Strips the slug suffix from an ID parameter (e.g., "abc123+my-page" -> "abc123").
 * Use this when extracting IDs from route params that may include slugs.
 */
export function stripSlug(idWithSlug: string): string {
  let s = idWithSlug;
  if (s.startsWith('@')) s = s.substring(1);
  const plusIndex = s.indexOf('+');
  return plusIndex >= 0 ? s.substring(0, plusIndex) : s;
}

/**
 * Converts text to a URL-friendly slug.
 */
export function slugify(text: string): string {
  return text
    .toString()
    .toLowerCase()
    .trim()
    .replace(/\s+/g, '-')
    .replace(/[^\w-]+/g, '')
    .replace(/--+/g, '-');
}

/**
 * Builds a workspace root URL.
 */
export function workspaceUrl(wsId: string, wsName?: string): string {
  const wsSlug = slugify(wsName || 'workspace');
  return `/w/@${wsId}+${wsSlug}/`;
}

/**
 * Builds a node (page/table) URL within a workspace.
 */
export function nodeUrl(wsId: string, wsName: string | undefined, nodeId: string, nodeTitle?: string): string {
  const wsSlug = slugify(wsName || 'workspace');
  const nodeSlug = nodeTitle ? slugify(nodeTitle) : '';
  return `/w/@${wsId}+${wsSlug}/@${nodeId}${nodeSlug ? '+' + nodeSlug : ''}`;
}

/**
 * Builds a workspace settings URL.
 */
export function workspaceSettingsUrl(wsId: string, wsName?: string): string {
  const wsSlug = slugify(wsName || 'workspace');
  return `/w/@${wsId}+${wsSlug}/settings`;
}

/**
 * Builds an organization settings URL.
 */
export function orgSettingsUrl(orgId: string, orgName?: string): string {
  const orgSlug = slugify(orgName || 'organization');
  return `/o/@${orgId}+${orgSlug}/settings`;
}

/** URL pattern matchers */

export interface WorkspaceMatch {
  wsId: string;
}

export interface NodeMatch {
  wsId: string;
  nodeId: string;
}

export interface SettingsMatch {
  id: string;
}

/**
 * Parses workspace root URL. Returns null if no match.
 */
export function parseWorkspaceRoot(path: string): WorkspaceMatch | null {
  const match = path.match(/^\/w\/@([^+/]+)(?:\+[^/]*)?\/?$/);
  if (match && match[1]) {
    return { wsId: match[1] };
  }
  return null;
}

/**
 * Parses workspace node URL. Returns null if no match.
 */
export function parseNodeUrl(path: string): NodeMatch | null {
  const match = path.match(/^\/w\/@([^+/]+)(?:\+[^/]*)?\/@([a-zA-Z0-9_-]+)(?:\+.*)?$/);
  if (match && match[1] && match[2]) {
    return { wsId: match[1], nodeId: match[2] };
  }
  return null;
}

/**
 * Parses workspace settings URL. Returns null if no match.
 */
export function parseWorkspaceSettings(path: string): SettingsMatch | null {
  const match = path.match(/^\/w\/@([^+/]+)(?:\+[^/]*)?\/settings\/?$/);
  if (match && match[1]) {
    return { id: match[1] };
  }
  return null;
}

/**
 * Parses organization settings URL. Returns null if no match.
 */
export function parseOrgSettings(path: string): SettingsMatch | null {
  const match = path.match(/^\/o\/@([^+/]+)(?:\+[^/]*)?\/settings\/?$/);
  if (match && match[1]) {
    return { id: match[1] };
  }
  return null;
}

/**
 * Checks if path is a static route.
 */
export function isStaticRoute(path: string): 'privacy' | 'terms' | 'profile' | 'settings' | null {
  if (path === '/privacy') return 'privacy';
  if (path === '/terms') return 'terms';
  if (path === '/profile') return 'profile';
  if (path.startsWith('/settings')) return 'settings';
  return null;
}

/** Unified Settings URL types */

export interface UnifiedSettingsMatch {
  type: 'profile' | 'workspace' | 'org' | 'server' | 'redirect';
  id?: string;
  section?: string;
}

/**
 * Builds a unified settings URL.
 */
export function settingsUrl(type?: 'user' | 'workspace' | 'org' | 'server', id?: string, name?: string): string {
  if (!type || type === 'user') return '/settings/user';
  if (type === 'server') return '/settings/server';
  const slug = slugify(name || type);
  return `/settings/${type}/@${id}+${slug}`;
}
