// Manages web push subscription lifecycle.

import type { APIClient } from '@sdk/api.gen';

function urlBase64ToUint8Array(base64String: string): Uint8Array {
  const padding = '='.repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(base64);
  const arr = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) {
    arr[i] = raw.charCodeAt(i);
  }
  return arr;
}

export async function subscribeToPush(
  registration: ServiceWorkerRegistration,
  vapidPublicKey: string,
  api: APIClient
): Promise<PushSubscription | null> {
  const permission = await Notification.requestPermission();
  if (permission !== 'granted') return null;

  const keyBytes = urlBase64ToUint8Array(vapidPublicKey);
  const subscription = await registration.pushManager.subscribe({
    userVisibleOnly: true,
    applicationServerKey: keyBytes.buffer as ArrayBuffer,
  });

  const json = subscription.toJSON();
  const endpoint = json.endpoint ?? '';
  const p256dh = json.keys?.p256dh ?? '';
  const auth = json.keys?.auth ?? '';
  if (!endpoint || !p256dh || !auth) return null;

  await api.notifications.subscribePush({ endpoint, p256dh, auth });

  return subscription;
}

export async function unsubscribeFromPush(subscription: PushSubscription, api: APIClient): Promise<void> {
  await api.notifications.unsubscribePush({ endpoint: subscription.endpoint });
  await subscription.unsubscribe();
}
