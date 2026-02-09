// Returns the active service worker registration (managed by VitePWA).

export async function registerServiceWorker(): Promise<ServiceWorkerRegistration | null> {
  if (!('serviceWorker' in navigator)) return null;
  try {
    return await navigator.serviceWorker.ready;
  } catch (err) {
    console.error('SW registration failed:', err);
    return null;
  }
}
