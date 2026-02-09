# Notification System Implementation Plan

## Overview

A generic notification system where users choose delivery channels (email and/or web push)
per notification type. Web push notifications use the
[Notifications API](https://developer.mozilla.org/en-US/docs/Web/API/Notifications_API)
delivered via a Service Worker.

### Design Principles

- **User-controlled**: Each user configures which notification types they want, and via which channels.
- **Channel-agnostic core**: The backend produces notification records; delivery is a separate concern dispatched per channel.
- **Additive**: No changes to existing handler signatures or wrapper types. Notification creation is called from within existing handlers.
- **Polling-first**: The frontend polls for new notifications. SSE/WebSocket is a future enhancement, not in scope.

### Notification Types (initial set)

| Type                  | Trigger                                    | Default channels   |
|-----------------------|--------------------------------------------|--------------------|
| `org_invite`          | User invited to an organization            | email + web        |
| `ws_invite`           | User invited to a workspace                | email + web        |
| `member_joined`       | A new member joins your org/workspace      | web                |
| `member_removed`      | A member is removed from your org/ws       | web                |
| `page_mention`        | Someone @-mentions you in a page           | email + web        |
| `page_edited`         | A page you follow is edited (future)       | web                |

New types can be added by defining a constant and registering a default preference.

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                     Frontend                        │
│                                                     │
│  NotificationContext ◄── poll GET /notifications ──►│
│       │                                             │
│       ├─ NotificationBell (badge + dropdown)        │
│       ├─ NotificationPanel (full list)              │
│       └─ Service Worker ◄── push via Notifications  │
│              │                    API               │
│              └─ POST /notifications/subscribe       │
│                 (saves PushSubscription)             │
└─────────────────────────────────────────────────────┘
                          │
                     HTTP/JSON
                          │
┌─────────────────────────────────────────────────────┐
│                     Backend                         │
│                                                     │
│  Existing handlers ──► NotificationService.Emit()   │
│                              │                      │
│                    ┌─────────┴──────────┐           │
│                    │  Dispatcher        │           │
│                    │  (per-user prefs)  │           │
│                    ├───────┬────────────┤           │
│                    ▼       ▼            ▼           │
│               DB store  Email svc  WebPush svc     │
│              (JSONL)   (SMTP)     (RFC 8030)       │
│                                                     │
│  New tables:                                        │
│    notifications.jsonl                              │
│    push_subscriptions.jsonl                         │
└─────────────────────────────────────────────────────┘
```

---

## Phase 1: Backend Storage & Preferences

### 1.1 Notification Entity

**File:** `backend/internal/storage/identity/notification.go`

```go
type NotificationType string

const (
    NotifOrgInvite     NotificationType = "org_invite"
    NotifWSInvite      NotificationType = "ws_invite"
    NotifMemberJoined  NotificationType = "member_joined"
    NotifMemberRemoved NotificationType = "member_removed"
    NotifPageMention   NotificationType = "page_mention"
    NotifPageEdited    NotificationType = "page_edited"
)

type Notification struct {
    ID         jsonldb.ID       `json:"id"`
    UserID     jsonldb.ID       `json:"user_id"`      // recipient
    Type       NotificationType `json:"type"`
    Title      string           `json:"title"`         // human-readable summary
    Body       string           `json:"body,omitempty"` // optional detail
    ResourceID string           `json:"resource_id,omitempty"` // e.g. org ID, ws ID, node ID
    ActorID    jsonldb.ID       `json:"actor_id,omitempty"` // who triggered it
    Read       bool             `json:"read"`
    Created    storage.Time     `json:"created"`
}
```

Implements `Row[*Notification]` (GetID, Clone, Validate).

**NotificationService** on the same file:
- `table *jsonldb.Table[*Notification]`, indexed by `UserID`.
- Methods: `Create`, `ListByUser(userID, limit, offset, unreadOnly)`, `MarkRead(id, userID)`, `MarkAllRead(userID)`, `Delete(id, userID)`, `DeleteOlderThan(cutoff)`.
- `CountUnread(userID) int` for badge count.

### 1.2 User Notification Preferences

**Extend** `UserSettings` in `backend/internal/storage/identity/user.go`:

```go
type UserSettings struct {
    Theme                string                     `json:"theme"`
    Language             string                     `json:"language"`
    LastActiveWorkspaces []jsonldb.ID               `json:"last_active_workspaces,omitempty"`
    NotificationPrefs    NotificationPreferences    `json:"notification_prefs,omitempty"`
}

type NotificationPreferences struct {
    // Per-type channel selection. Missing key = use DefaultChannels.
    Overrides map[NotificationType]ChannelSet `json:"overrides,omitempty"`
}

type ChannelSet struct {
    Email bool `json:"email"`
    Web   bool `json:"web"` // in-app + browser push
}
```

A helper `EffectiveChannels(notifType) ChannelSet` returns the override if present, otherwise the hard-coded default for that type (see table above).

### 1.3 Push Subscription Entity

**File:** `backend/internal/storage/identity/push_subscription.go`

```go
type PushSubscription struct {
    ID       jsonldb.ID   `json:"id"`
    UserID   jsonldb.ID   `json:"user_id"`
    Endpoint string       `json:"endpoint"`  // Web Push endpoint URL
    P256dh   string       `json:"p256dh"`    // client public key (base64url)
    Auth     string       `json:"auth"`      // auth secret (base64url)
    Created  storage.Time `json:"created"`
}
```

**PushSubscriptionService**: CRUD by UserID, `DeleteByEndpoint` for unsubscribe, `ListByUser`.

### 1.4 VAPID Key Pair

Store the server's VAPID key pair in `ServerConfig` (generated once on first boot if missing):

```go
type VAPIDConfig struct {
    PublicKey  string `json:"vapid_public_key"`
    PrivateKey string `json:"vapid_private_key"`
}
```

Generated via `ecdsa.GenerateKey(elliptic.P256(), rand.Reader)` and stored base64url-encoded.

---

## Phase 2: Backend Dispatch & API

### 2.1 Notification Dispatcher

**File:** `backend/internal/server/handlers/notify.go`

Central function called from existing handlers:

```go
func (svc *Services) Emit(ctx context.Context, userID jsonldb.ID, notifType NotificationType, title, body, resourceID string, actorID jsonldb.ID) {
    // 1. Look up user's EffectiveChannels(notifType)
    // 2. Always persist to notifications table (for in-app list)
    // 3. If channels.Email && svc.Email != nil → send email (async, fire-and-forget with logging)
    // 4. If channels.Web → for each PushSubscription of user → send Web Push (async)
}
```

Email and Web Push sends are fire-and-forget goroutines with error logging — notification delivery must never block or fail the original request.

### 2.2 Web Push Sending

Use the [webpush-go](https://github.com/SherClockHolmes/webpush-go) library (RFC 8030 + VAPID):

```go
import webpush "github.com/SherClockHolmes/webpush-go"

func sendWebPush(sub *identity.PushSubscription, payload []byte, vapidPublicKey, vapidPrivateKey string) error {
    _, err := webpush.SendNotification(payload, &webpush.Subscription{
        Endpoint: sub.Endpoint,
        Keys: webpush.Keys{
            P256dh: sub.P256dh,
            Auth:   sub.Auth,
        },
    }, &webpush.Options{
        VAPIDPublicKey:  vapidPublicKey,
        VAPIDPrivateKey: vapidPrivateKey,
        TTL:             86400,
    })
    return err
}
```

On 410 Gone response, auto-delete the subscription.

### 2.3 API Endpoints

All wrapped with `WrapAuth` (user-scoped, no org/ws context needed).

| Method | Path                                    | Handler                     | Description                         |
|--------|-----------------------------------------|-----------------------------|-------------------------------------|
| GET    | `/api/v1/notifications`                 | `ListNotifications`         | List with `?limit=&offset=&unread=` |
| GET    | `/api/v1/notifications/unread-count`    | `GetUnreadCount`            | Returns `{ count: N }`             |
| POST   | `/api/v1/notifications/{id}/read`       | `MarkNotificationRead`      | Mark single as read                 |
| POST   | `/api/v1/notifications/read-all`        | `MarkAllNotificationsRead`  | Mark all as read                    |
| POST   | `/api/v1/notifications/{id}/delete`     | `DeleteNotification`        | Delete single notification          |
| GET    | `/api/v1/notifications/preferences`     | `GetNotificationPrefs`      | Get user's channel preferences      |
| POST   | `/api/v1/notifications/preferences`     | `UpdateNotificationPrefs`   | Update channel preferences          |
| GET    | `/api/v1/notifications/vapid-key`       | `GetVAPIDPublicKey`         | Public key for push subscription    |
| POST   | `/api/v1/notifications/subscribe`       | `SubscribePush`             | Save push subscription              |
| POST   | `/api/v1/notifications/unsubscribe`     | `UnsubscribePush`           | Remove push subscription            |

### 2.4 DTO Types

**File:** `backend/internal/server/dto/types.go` (append)

```go
type NotificationType = string // tygo: generates as string union

type NotificationDTO struct {
    ID         jsonldb.ID `json:"id"`
    Type       string     `json:"type"`
    Title      string     `json:"title"`
    Body       string     `json:"body,omitempty"`
    ResourceID string     `json:"resource_id,omitempty"`
    ActorID    jsonldb.ID `json:"actor_id,omitempty"`
    ActorName  string     `json:"actor_name,omitempty"` // denormalized for display
    Read       bool       `json:"read"`
    CreatedAt  Time       `json:"created_at"`
}

type ChannelSetDTO struct {
    Email bool `json:"email"`
    Web   bool `json:"web"`
}

type NotificationPrefsDTO struct {
    Defaults  map[string]ChannelSetDTO `json:"defaults"`  // type → default channels
    Overrides map[string]ChannelSetDTO `json:"overrides"` // type → user overrides
}
```

**Request/Response types** in `dto/request.go` and `dto/response.go`:

```go
// request.go
type ListNotificationsRequest struct {
    Limit      int  `json:"limit,omitempty"`
    Offset     int  `json:"offset,omitempty"`
    UnreadOnly bool `json:"unread_only,omitempty"`
}

type UpdateNotificationPrefsRequest struct {
    Overrides map[string]ChannelSetDTO `json:"overrides"`
}

type PushSubscribeRequest struct {
    Endpoint string `json:"endpoint"`
    P256dh   string `json:"p256dh"`
    Auth     string `json:"auth"`
}

type PushUnsubscribeRequest struct {
    Endpoint string `json:"endpoint"`
}

// response.go
type ListNotificationsResponse struct {
    Notifications []NotificationDTO `json:"notifications"`
    Total         int               `json:"total"`
    UnreadCount   int               `json:"unread_count"`
}

type UnreadCountResponse struct {
    Count int `json:"count"`
}

type VAPIDKeyResponse struct {
    PublicKey string `json:"public_key"`
}
```

Run `make types` to generate `sdk/types.gen.ts`.

### 2.5 Integration Points (Emit Calls)

Add `svc.Emit(...)` calls in existing handlers:

| Handler file          | Function                 | Notification type   |
|-----------------------|--------------------------|---------------------|
| `invitations.go`      | `InviteToOrg`            | `org_invite`        |
| `invitations.go`      | `InviteToWorkspace`      | `ws_invite`         |
| `memberships.go`      | `AddOrgMember` (accept)  | `member_joined`     |
| `memberships.go`      | `RemoveOrgMember`        | `member_removed`    |
| `memberships.go`      | `AddWSMember` (accept)   | `member_joined`     |
| `memberships.go`      | `RemoveWSMember`         | `member_removed`    |

`page_mention` requires @-mention parsing (Phase 4 future work, not in initial scope).

### 2.6 Wiring

1. Add `Notification *identity.NotificationService` and `PushSubscription *identity.PushSubscriptionService` to `handlers.Services`.
2. Initialize both in `cmd/mddb/main.go` when opening the data directory.
3. Add `NotificationHandler` struct in `handlers/notifications.go`.
4. Register routes in `router.go`.
5. Run `go generate ./internal/server/` to update `apiroutes` and `apiclient`.

---

## Phase 3: Frontend — Service Worker & Push Subscription

### 3.1 Service Worker

**File:** `frontend/public/sw.js`

Minimal service worker that:
1. Listens for `push` events and shows a browser notification via `self.registration.showNotification()`.
2. Listens for `notificationclick` events and focuses/opens the app window at the relevant URL.
3. No caching strategy (the app is not offline-first yet; this SW exists solely for push).

```js
// frontend/public/sw.js
self.addEventListener('push', (event) => {
  const data = event.data?.json() ?? {};
  const title = data.title || 'mddb';
  const options = {
    body: data.body || '',
    icon: '/icon-192.png',
    badge: '/favicon.png',
    tag: data.id,           // collapse duplicate notifications
    data: { url: data.url } // for click handler
  };
  event.waitUntil(self.registration.showNotification(title, options));
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const url = event.notification.data?.url || '/';
  event.waitUntil(
    clients.matchAll({ type: 'window' }).then((windowClients) => {
      for (const client of windowClients) {
        if (client.url.includes(url) && 'focus' in client) return client.focus();
      }
      return clients.openWindow(url);
    })
  );
});
```

### 3.2 Service Worker Registration

**File:** `frontend/src/notifications/sw-register.ts`

```ts
export async function registerServiceWorker(): Promise<ServiceWorkerRegistration | null> {
  if (!('serviceWorker' in navigator)) return null;
  try {
    return await navigator.serviceWorker.register('/sw.js');
  } catch (err) {
    console.error('SW registration failed:', err);
    return null;
  }
}
```

Called once from `NotificationProvider` on mount.

### 3.3 Push Subscription Manager

**File:** `frontend/src/notifications/push-manager.ts`

```ts
export async function subscribeToPush(
  registration: ServiceWorkerRegistration,
  vapidPublicKey: string,
  api: ApiClient,
): Promise<PushSubscription | null> {
  const permission = await Notification.requestPermission();
  if (permission !== 'granted') return null;

  const subscription = await registration.pushManager.subscribe({
    userVisibleOnly: true,
    applicationServerKey: urlBase64ToUint8Array(vapidPublicKey),
  });

  const json = subscription.toJSON();
  await api.notifications.subscribe({
    endpoint: json.endpoint!,
    p256dh: json.keys!.p256dh!,
    auth: json.keys!.auth!,
  });

  return subscription;
}

export async function unsubscribeFromPush(
  subscription: PushSubscription,
  api: ApiClient,
): Promise<void> {
  await api.notifications.unsubscribe({ endpoint: subscription.endpoint });
  await subscription.unsubscribe();
}
```

### 3.4 Vite Config

Add `sw.js` to the build pipeline. Since it's in `public/`, Vite copies it as-is (no bundling needed). No config changes required.

---

## Phase 4: Frontend — UI Components

### 4.1 NotificationContext

**File:** `frontend/src/contexts/NotificationContext.tsx`

```ts
interface NotificationContextValue {
  notifications: Accessor<NotificationDTO[]>;
  unreadCount: Accessor<number>;
  isLoading: Accessor<boolean>;
  pushEnabled: Accessor<boolean>;
  markAsRead: (id: string) => Promise<void>;
  markAllAsRead: () => Promise<void>;
  deleteNotification: (id: string) => Promise<void>;
  enablePush: () => Promise<boolean>;  // returns success
  disablePush: () => Promise<void>;
  refresh: () => Promise<void>;
}
```

- Polls `GET /notifications/unread-count` every 60 seconds.
- Full list fetched lazily when the panel opens.
- Manages SW registration and push subscription lifecycle.
- Provided at the `App.tsx` root level, inside `AuthProvider`.

### 4.2 NotificationBell

**File:** `frontend/src/components/NotificationBell.tsx` + `.module.css`

- Bell icon (Material Symbols `notifications`) in the app header.
- Red badge with unread count (hidden when 0).
- Click opens the NotificationPanel dropdown.
- Keyboard accessible (Enter/Space to toggle, Escape to close).

### 4.3 NotificationPanel

**File:** `frontend/src/components/NotificationPanel.tsx` + `.module.css`

- Dropdown anchored to the bell.
- Header: "Notifications" + "Mark all as read" button.
- List of `NotificationItem` components (icon per type, title, body, relative time, read/unread dot).
- Click on item → navigate to resource, mark as read.
- Empty state: "No notifications" message.
- "Load more" pagination at the bottom.

### 4.4 Notification Preferences UI

**File:** `frontend/src/components/settings/NotificationSettings.tsx` + `.module.css`

Located in the user settings panel (`/settings/user`).

- Table with one row per notification type.
- Two toggle columns: Email, Web.
- "Enable browser notifications" button that calls `Notification.requestPermission()` and registers push subscription.
- Shows current push permission status (`granted`/`denied`/`default`).
- If push is `denied`, show a note explaining how to re-enable in browser settings.

### 4.5 Header Integration

Add `<NotificationBell />` to the workspace layout header (in `WorkspaceLayout.tsx`), next to the user menu.

### 4.6 i18n Keys

Add a `notifications` section to `i18n/types.ts` and all 4 dictionaries:

```ts
notifications: {
  title: string;
  markAllRead: string;
  empty: string;
  loadMore: string;
  enablePush: string;
  pushDenied: string;
  pushEnabled: string;
  preferences: string;
  channelEmail: string;
  channelWeb: string;
  // Per-type display names
  typeOrgInvite: string;
  typeWsInvite: string;
  typeMemberJoined: string;
  typeMemberRemoved: string;
  typePageMention: string;
  typePageEdited: string;
};
```

---

## Phase 5: Notification Cleanup

### 5.1 Retention Garbage Collection

Add a background goroutine in `cmd/mddb/main.go` that runs daily:
- Delete notifications older than 90 days (configurable via `ServerConfig`).
- Delete push subscriptions that have returned 410 Gone.
- Cap per-user notification count at 500 (delete oldest).

### 5.2 Quota

Add to `ServerConfig.Quotas`:
```go
NotificationRetentionDays  int `json:"notification_retention_days"`  // default 90
MaxNotificationsPerUser    int `json:"max_notifications_per_user"`   // default 500
```

---

## File Manifest

### New Files

| File | Purpose |
|------|---------|
| `backend/internal/storage/identity/notification.go` | Notification entity, service, JSONL table |
| `backend/internal/storage/identity/push_subscription.go` | PushSubscription entity, service |
| `backend/internal/server/handlers/notifications.go` | HTTP handlers for notification endpoints |
| `backend/internal/server/handlers/notify.go` | `Emit()` dispatcher (email + web push) |
| `frontend/public/sw.js` | Service Worker for push events |
| `frontend/src/notifications/sw-register.ts` | SW registration helper |
| `frontend/src/notifications/push-manager.ts` | Push subscription lifecycle |
| `frontend/src/contexts/NotificationContext.tsx` | SolidJS notification state |
| `frontend/src/components/NotificationBell.tsx` | Bell icon + badge |
| `frontend/src/components/NotificationBell.module.css` | Bell styles |
| `frontend/src/components/NotificationPanel.tsx` | Notification dropdown list |
| `frontend/src/components/NotificationPanel.module.css` | Panel styles |
| `frontend/src/components/settings/NotificationSettings.tsx` | Preferences UI |
| `frontend/src/components/settings/NotificationSettings.module.css` | Preferences styles |

### Modified Files

| File | Change |
|------|--------|
| `backend/internal/storage/identity/user.go` | Add `NotificationPreferences` to `UserSettings` |
| `backend/internal/storage/config.go` | Add `VAPIDConfig`, notification quota fields |
| `backend/internal/server/handlers/services.go` | Add `Notification`, `PushSubscription` services |
| `backend/internal/server/router.go` | Register notification routes |
| `backend/internal/server/dto/types.go` | Add notification DTOs |
| `backend/internal/server/dto/request.go` | Add notification requests |
| `backend/internal/server/dto/response.go` | Add notification responses |
| `backend/internal/server/handlers/invitations.go` | Add `Emit()` calls |
| `backend/internal/server/handlers/memberships.go` | Add `Emit()` calls |
| `backend/internal/email/templates.go` | Add notification email template |
| `backend/cmd/mddb/main.go` | Init notification services, start GC goroutine |
| `frontend/src/App.tsx` | Add `NotificationProvider` |
| `frontend/src/sections/WorkspaceLayout.tsx` | Add `NotificationBell` to header |
| `frontend/src/components/settings/ProfileSettings.tsx` | Add link/section for notification prefs |
| `frontend/public/manifest.json` | No changes needed (icons already present) |
| `frontend/src/i18n/types.ts` | Add `notifications` section |
| `frontend/src/i18n/dictionaries/en.ts` | Add English translations |
| `frontend/src/i18n/dictionaries/fr.ts` | Add French translations |
| `frontend/src/i18n/dictionaries/de.ts` | Add German translations |
| `frontend/src/i18n/dictionaries/es.ts` | Add Spanish translations |
| `sdk/types.gen.ts` | Regenerated (DO NOT EDIT) |
| `sdk/api.gen.ts` | Regenerated (DO NOT EDIT) |
| `sdk/API.md` | Regenerated (DO NOT EDIT) |

---

## Implementation Order

1. **Phase 1** — Backend storage (notification table, push subscription table, user prefs extension). Unit tests.
2. **Phase 2** — Backend API endpoints + dispatcher. Integration tests. `make types` to generate TS types.
3. **Phase 3** — Service worker + push subscription frontend helpers.
4. **Phase 4** — UI components (bell, panel, preferences). i18n. Wire into app.
5. **Phase 5** — GC goroutine, retention limits.
6. **Verify** — `make lint build test`, manual E2E test of full flow.

---

## Dependencies

| Dependency | Purpose | License |
|------------|---------|---------|
| [webpush-go](https://github.com/SherClockHolmes/webpush-go) | RFC 8030 Web Push + VAPID signing | MIT |

No new frontend dependencies. The Notifications API is a browser built-in.

---

## Out of Scope (future work)

- **SSE/WebSocket** for real-time push (currently polling).
- **@-mention parsing** in ProseMirror → `page_mention` notifications.
- **Page follow/watch** system → `page_edited` notifications.
- **Notification grouping/batching** (e.g. "5 new members joined").
- **Digest emails** (daily/weekly summary).
- **Admin notification management** (broadcast to all users).
