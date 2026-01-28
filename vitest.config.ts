import { defineConfig } from "vitest/config";
import solid from "vite-plugin-solid";
import { resolve } from 'path';

export default defineConfig({
  root: 'frontend',
  cacheDir: '../node_modules/.vite',
  resolve: {
    alias: {
      '@sdk': resolve(__dirname, 'sdk'),
    },
  },
  plugins: [solid()],
  test: {
    environment: "jsdom",
    silent: "passed-only",
    reporters: ["dot"],
    coverage: {
      provider: "v8",
      reporter: ["text", "lcov"],
      reportsDirectory: "../coverage",
    },
  },
});
