// Vitest test setup with jsdom polyfills (scrollTo stub) and jest-dom matchers.
import { vi } from 'vitest';
import '@testing-library/jest-dom/vitest';

// JSDOM doesn't implement scrollTo, which causes tests to fail or log errors
// when using components that scroll (like the editor or sidebars).
if (typeof window !== 'undefined') {
  window.scrollTo = vi.fn();
}
