// Vite build configuration for the SolidJS frontend.
import { defineConfig } from 'vite';
import solid from 'vite-plugin-solid';
import solidSVG from 'vite-solid-svg';
import { VitePWA } from 'vite-plugin-pwa';
import { visualizer } from 'rollup-plugin-visualizer';
import { resolve } from 'path';

export default defineConfig({
  root: 'frontend',
  cacheDir: '../node_modules/.vite',
  resolve: {
    // A duplicated solid-js copy breaks reactivity signal identity at runtime.
    dedupe: ['solid-js'],
    alias: {
      '@sdk': resolve(import.meta.dirname, 'sdk'),
    },
  },
  plugins: [
    solid(),
    solidSVG(),
    VitePWA({
      strategies: 'injectManifest',
      srcDir: 'src',
      filename: 'sw.ts',
      registerType: 'autoUpdate',
      includeAssets: ['favicon.png', 'apple-touch-icon.png', 'icon.svg'],
      manifest: {
        name: 'mddb - Markdown Document & Table',
        short_name: 'mddb',
        description: 'A markdown-based document and table application',
        theme_color: '#1a1a1a',
        background_color: '#ffffff',
        display: 'standalone',
        start_url: '/',
        icons: [
          {
            src: 'icon-192.png',
            sizes: '192x192',
            type: 'image/png',
          },
          {
            src: 'icon-512.png',
            sizes: '512x512',
            type: 'image/png',
          },
          {
            src: 'icon-512.png',
            sizes: '512x512',
            type: 'image/png',
            purpose: 'maskable',
          },
        ],
      },
      injectManifest: {
        globPatterns: ['**/*.{js,css,html,png,svg,ico,woff,woff2}'],
        buildPlugins: {
          vite: [
            {
              name: 'sw-quiet',
              // The plugin still asks for the deprecated rollup spelling of
              // "bundle the service worker into one file".
              config: (config) => {
                const output = config.build?.rollupOptions?.output;
                if (output && !Array.isArray(output)) {
                  delete output.inlineDynamicImports;
                  output.codeSplitting = false;
                }
                return { build: { reportCompressedSize: false }, logLevel: 'silent' };
              },
            },
          ],
        },
      },
    }),
    visualizer({
      filename: 'bundle-stats.html',
      open: false,
      gzipSize: true,
      brotliSize: true,
    }),
  ],
  build: {
    // Matches the browsers Vite 8 supports by default; pinned so a Vite upgrade
    // cannot silently move the floor.
    target: 'baseline-widely-available',
    minify: 'oxc',
    cssMinify: 'lightningcss',
    reportCompressedSize: false,
    outDir: '../backend/frontend/dist', // relative to frontend/
    emptyOutDir: true,
    // dist/ is brotli-compressed into the Go binary, so a source map would be
    // embedded as dead weight. Debug against the dev server instead.
    sourcemap: false,
    rolldownOptions: {
      output: {
        // Split chunks for better caching and lazy loading
        manualChunks: (id) => {
          if (id.includes('node_modules')) {
            // ProseMirror and markdown-it are lazy-loaded with Editor
            if (
              id.includes('prosemirror') ||
              id.includes('markdown-it') ||
              id.includes('entities') ||
              id.includes('linkify-it') ||
              id.includes('mdurl') ||
              id.includes('uc.micro') ||
              id.includes('punycode')
            ) {
              return 'editor-vendor';
            }
            // Core dependencies always loaded
            return 'vendor';
          }
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/assets': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
});
