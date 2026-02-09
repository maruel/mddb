// Service worker: Workbox precaching + web push notification handlers.

import { precacheAndRoute } from 'workbox-precaching';

declare const self: ServiceWorkerGlobalScope;

// Workbox injects the precache manifest here at build time.
precacheAndRoute(self.__WB_MANIFEST);

// --- Web Push ---

self.addEventListener('push', (event) => {
  const data = event.data?.json() ?? {};
  const title = data.title;
  const options: NotificationOptions = {
    body: data.body || '',
    icon: '/icon-192.png',
    badge: '/favicon.png',
    tag: data.id,
    data: { url: data.url },
  };
  event.waitUntil(self.registration.showNotification(title, options));
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const url = (event.notification.data as { url?: string })?.url || '/';
  event.waitUntil(
    self.clients.matchAll({ type: 'window' }).then((windowClients) => {
      for (const client of windowClients) {
        if (client.url.includes(url) && 'focus' in client) return client.focus();
      }
      return self.clients.openWindow(url);
    })
  );
});
