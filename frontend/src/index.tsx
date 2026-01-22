import { render } from 'solid-js/web';
import { I18nProvider, type Locale } from './i18n';
import './variables.css';
import App from './App';

// Get initial locale from localStorage or default to 'en'
const storedLocale = localStorage.getItem('mddb_locale') as Locale | null;
const initialLocale: Locale = storedLocale && ['en', 'fr', 'de', 'es'].includes(storedLocale) ? storedLocale : 'en';

const root = document.getElementById('app');
if (root) {
  render(
    () => (
      <I18nProvider initialLocale={initialLocale}>
        <App />
      </I18nProvider>
    ),
    root
  );
}
