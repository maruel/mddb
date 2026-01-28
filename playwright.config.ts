import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: [
    ['html', { open: 'never' }],
    ['json', { outputFile: 'playwright-report/results.json' }],
  ],
  use: {
    baseURL: 'http://localhost:8080',
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'], channel: 'chrome' },
    },
  ],
  // Start the server before running tests with isolated test data directory
  // Server logs are captured to data-e2e/server.log (copied to report by make e2e)
  // TEST_FAST_RATE_LIMIT defaults to 1 (fast mode) unless explicitly set to 0
  webServer: {
    command: `rm -rf ./data-e2e && mkdir -p ./data-e2e && TEST_OAUTH=1 TEST_FAST_RATE_LIMIT=${process.env.TEST_FAST_RATE_LIMIT ?? '1'} DATA_DIR=./data-e2e make dev > ./data-e2e/server.log 2>&1`,
    url: 'http://localhost:8080/api/health',
    reuseExistingServer: false, // Always start fresh to ensure TEST_OAUTH is set
    timeout: 30000,
  },
});
