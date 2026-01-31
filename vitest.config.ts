import { defineConfig } from "vitest/config";
import solid from "vite-plugin-solid";
import solidSVG from "vite-solid-svg";
import { resolve } from 'path';

export default defineConfig({
  root: 'frontend',
  cacheDir: '../node_modules/.vite',
  resolve: {
    alias: {
      '@sdk': resolve(__dirname, 'sdk'),
    },
  },
  plugins: [solid(), solidSVG()],
  test: {
    environment: "jsdom",
    setupFiles: ["src/test-setup.ts"],
    silent: "passed-only",
    reporters: ["dot"],
    coverage: {
      provider: "v8",
      reporter: ["text", "lcov"],
      reportsDirectory: "../coverage",
    },
    server: {
      deps: {
        inline: [/@solidjs\/router/],
      },
    },
  },
});
