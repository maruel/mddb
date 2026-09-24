// Application entry point rendering the root component.

import { render } from "solid-js/web";
import { I18nProvider, type Locale } from "./i18n";
import "./global.css";
import App from "./App";

// Get initial locale from localStorage or default to 'en'
const storedLocale = localStorage.getItem("mddb_locale") as Locale | null;
const initialLocale: Locale = storedLocale && ["en", "fr", "de", "es"].includes(storedLocale) ? storedLocale : "en";

const root = document.getElementById("app");
if (root) {
  render(
    () => (
      <I18nProvider initialLocale={initialLocale}>
        <App />
      </I18nProvider>
    ),
    root,
  );
}

// Reload once when an updated service worker takes control, so the page runs the
// new hashed bundle instead of mixing it with chunks the new build removed.
if ("serviceWorker" in navigator) {
  let reloaded = false;
  navigator.serviceWorker.addEventListener("controllerchange", () => {
    if (reloaded) return;
    reloaded = true;
    window.location.reload();
  });
}
