// Application entry point rendering the root component.

import { render } from "solid-js/web";
import { I18nProvider } from "./i18n";
import "./global.css";
import App from "./App";

const root = document.getElementById("app");
if (root) {
  render(
    () => (
      <I18nProvider>
        <App />
      </I18nProvider>
    ),
    root,
  );
}

// Reload once when an updated service worker takes control, so the page runs the
// new hashed bundle instead of mixing it with chunks the new build removed.
if ("serviceWorker" in navigator) {
  let hasController = navigator.serviceWorker.controller !== null;
  let reloaded = false;
  navigator.serviceWorker.addEventListener("controllerchange", () => {
    if (!hasController) {
      hasController = true;
      return;
    }
    if (reloaded) return;
    reloaded = true;
    window.location.reload();
  });
}
