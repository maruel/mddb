// Main application component with router setup.

import { lazy, Show, Switch, Match, Suspense, type ParentComponent } from 'solid-js';
import { Router, Route, Navigate, A } from '@solidjs/router';
import { AuthProvider, useAuth } from './contexts';
import AppErrorBoundary from './components/ErrorBoundary';
import PWAInstallBanner from './components/PWAInstallBanner';

// Lazy-loaded route components
const Auth = lazy(() => import('./components/Auth'));
const Privacy = lazy(() => import('./components/Privacy'));
const Terms = lazy(() => import('./components/Terms'));
const Onboarding = lazy(() => import('./sections/Onboarding'));

// Import settings section and route components (not lazy - needed for nested routes)
import SettingsSection, {
  ProfileSettingsRoute,
  WorkspaceSettingsRoute,
  OrgSettingsRoute,
  ServerSettingsGuard,
  SettingsRedirect,
} from './sections/SettingsSection';

// Import workspace section and route components (not lazy - needed for nested routes)
import WorkspaceSection, { WorkspaceLayout, WorkspaceRoot, NodeView } from './sections/WorkspaceSection';

// Loading fallback for lazy-loaded routes
function RouteLoading() {
  return <div style={{ padding: '2rem', 'text-align': 'center' }}>Loading...</div>;
}

// 404 page for unmatched routes - shows explicit error instead of silent redirect
function NotFound() {
  const path = window.location.pathname;
  console.error('404: Route not found:', path);
  return (
    <div style={{ padding: '2rem', 'text-align': 'center' }}>
      <h1>Page Not Found</h1>
      <p>
        The path <code>{path}</code> doesn't exist.
      </p>
      <A href="/login">Go to login</A>
    </div>
  );
}

// Route guard that redirects unauthenticated users to /login
const RequireAuth: ParentComponent = (props) => {
  const { user, ready } = useAuth();

  // Wait for auth check to complete before making decisions
  return (
    <Show when={ready()} fallback={<RouteLoading />}>
      <Show when={user()} fallback={<Navigate href="/login" />}>
        {props.children}
      </Show>
    </Show>
  );
};

// Auth route that redirects authenticated users away from login
function AuthRoute() {
  const { user, login, ready } = useAuth();

  // Wait for auth check to complete before making decisions
  return (
    <Switch fallback={<RouteLoading />}>
      <Match when={ready() && user()?.workspace_id}>
        <Navigate href={`/w/${user()?.workspace_id}`} />
      </Match>
      <Match when={ready() && user() && !user()?.workspace_id}>
        <Navigate href="/onboarding" />
      </Match>
      <Match when={ready() && !user()}>
        <Suspense fallback={<RouteLoading />}>
          <Auth onLogin={login} />
        </Suspense>
      </Match>
    </Switch>
  );
}

// Root redirect - send to login or workspace based on auth state
function RootRedirect() {
  const { user, ready } = useAuth();

  // Wait for auth check to complete before making decisions
  // Use Switch/Match for more explicit control over which Navigate renders
  return (
    <Switch fallback={<RouteLoading />}>
      <Match when={ready() && user()?.workspace_id}>
        <Navigate href={`/w/${user()?.workspace_id}`} />
      </Match>
      <Match when={ready() && user() && !user()?.workspace_id}>
        {/* User is authenticated but has no workspace - send to onboarding */}
        <Navigate href="/onboarding" />
      </Match>
      <Match when={ready() && !user()}>
        <Navigate href="/login" />
      </Match>
    </Switch>
  );
}

// Settings section wrapper with auth guard
const SettingsSectionWithAuth: ParentComponent = (props) => {
  return (
    <RequireAuth>
      <Suspense fallback={<RouteLoading />}>
        <SettingsSection>{props.children}</SettingsSection>
      </Suspense>
    </RequireAuth>
  );
};

// Workspace section wrapper with auth guard
const WorkspaceSectionWithAuth: ParentComponent = (props) => {
  return (
    <RequireAuth>
      <Suspense fallback={<RouteLoading />}>
        <WorkspaceSection>{props.children}</WorkspaceSection>
      </Suspense>
    </RequireAuth>
  );
};

// App routes wrapped in router
function AppRoutes() {
  return (
    <Router explicitLinks>
      {/* Public routes */}
      <Route path="/login" component={AuthRoute} />
      <Route
        path="/onboarding"
        component={() => (
          <RequireAuth>
            <Suspense fallback={<RouteLoading />}>
              <Onboarding />
            </Suspense>
          </RequireAuth>
        )}
      />
      <Route
        path="/privacy"
        component={() => (
          <Suspense fallback={<RouteLoading />}>
            <Privacy />
          </Suspense>
        )}
      />
      <Route
        path="/terms"
        component={() => (
          <Suspense fallback={<RouteLoading />}>
            <Terms />
          </Suspense>
        )}
      />

      {/* Protected settings routes with nested routing */}
      <Route path="/settings" component={SettingsSectionWithAuth}>
        <Route path="/" component={SettingsRedirect} />
        <Route path="/user" component={ProfileSettingsRoute} />
        <Route path="/server" component={ServerSettingsGuard} />
        <Route path="/workspace/:wsId" component={WorkspaceSettingsRoute} />
        <Route path="/org/:orgId" component={OrgSettingsRoute} />
      </Route>

      {/* Protected workspace routes with nested routing */}
      <Route path="/w/:wsId" component={WorkspaceSectionWithAuth}>
        <Route path="/" component={WorkspaceLayout}>
          <Route path="/" component={WorkspaceRoot} />
          <Route path="/:nodeId" component={NodeView} />
        </Route>
      </Route>

      {/* Root redirect */}
      <Route path="/" component={RootRedirect} />

      {/* Catch-all 404 - shows explicit error instead of silent redirect */}
      <Route path="*" component={NotFound} />
    </Router>
  );
}

// Root component that provides global context
export default function App() {
  return (
    <AppErrorBoundary>
      <AuthProvider>
        <AppRoutes />
        <PWAInstallBanner />
      </AuthProvider>
    </AppErrorBoundary>
  );
}
