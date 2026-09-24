// Service worker: Workbox asset precaching, network-first documents, and web push handlers.

import { precacheAndRoute } from "workbox-precaching";

declare const self: ServiceWorkerGlobalScope;

// Workbox injects the precache manifest here at build time. The document is
// deliberately not precached (see injectManifest.globPatterns in vite.config.ts):
// index.html names the hashed bundle, so serving it cache-first pins the app to
// the build that installed the worker. Navigations are handled below instead.
precacheAndRoute(self.__WB_MANIFEST);

const documentCache = "mddb-documents";

// Fall back to the cached document once the network is this slow, so an offline
// or flaky connection shows the last shell instead of hanging on a blank page.
const documentTimeoutMS = 3000;

// Serve navigations network-first so a reload always picks up the current
// document, and with it the current hashed bundle. The last good document stays
// cached as the offline fallback.
self.addEventListener("fetch", (event) => {
  const { request } = event;
  if (request.method !== "GET" || request.mode !== "navigate") return;
  event.respondWith(
    (async () => {
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), documentTimeoutMS);
      try {
        const response = await fetch(request, { signal: controller.signal });
        if (response.ok) {
          const cache = await caches.open(documentCache);
          await cache.put("/index.html", response.clone());
        }
        return response;
      } catch (error) {
        const cached = await caches.match("/index.html");
        if (cached) return cached;
        throw error;
      } finally {
        clearTimeout(timer);
      }
    })(),
  );
});

// Activate an updated worker immediately and take control of open pages, so a
// deploy does not wait for every app tab to close before the new build loads.
self.addEventListener("install", () => {
  void self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener("message", (event) => {
  if ((event.data as { type?: string } | null)?.type === "SKIP_WAITING") {
    void self.skipWaiting();
  }
});

// --- Web Push ---

self.addEventListener("push", (event) => {
  const data = event.data?.json() ?? {};
  const title = data.title;
  const options: NotificationOptions = {
    body: data.body || "",
    icon: "/icon-192.png",
    badge: "/favicon.png",
    tag: data.id,
    data: { url: data.url },
  };
  event.waitUntil(self.registration.showNotification(title, options));
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url = (event.notification.data as { url?: string })?.url || "/";
  event.waitUntil(
    self.clients.matchAll({ type: "window" }).then((windowClients) => {
      for (const client of windowClients) {
        if (client.url.includes(url) && "focus" in client) return client.focus();
      }
      return self.clients.openWindow(url);
    }),
  );
});
