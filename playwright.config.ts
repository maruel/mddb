// Playwright end-to-end test configuration.
import { defineConfig, devices, type PlaywrightTestConfig } from "@playwright/test";

// playwrightConfig builds the e2e suite configuration. fastRateLimit adds the
// e2e-only -fast-rate-limit flag to the webServer's scripts/run-dev.py, which
// multiplies every rate limit 10000x: the parallel suite needs that, while the
// slow suite in playwright.slow.config.ts runs with production limits.
export function playwrightConfig(fastRateLimit: boolean): PlaywrightTestConfig {
  return {
    testDir: "./e2e",
    fullyParallel: true,
    forbidOnly: !!process.env.CI,
    retries: 0,
    reporter: [
      ["html", { open: "never" }],
      ["json", { outputFile: "playwright-report/results.json" }],
    ],
    use: {
      baseURL: "http://localhost:8080",
      trace: "on-first-retry",
    },
    projects: [
      {
        name: "chromium",
        use: { ...devices["Desktop Chrome"], channel: "chrome" },
      },
    ],
    // Start the server before running tests with isolated test data directory.
    // scripts/run-dev.py --fake builds with the e2e tag (fake OAuth providers
    // and a raised user quota) and writes a temporary config; it keeps
    // production rate limits when --production-rate-limits is passed, never an
    // environment variable. Server logs are captured to data-e2e/server.log
    // (copied to report by make e2e).
    webServer: {
      command: `python3 scripts/clean_data_e2e.py && ./scripts/run-dev.py --data-dir ./data-e2e --fake${fastRateLimit ? "" : " --production-rate-limits"} > ./data-e2e/server.log 2>&1`,
      url: "http://localhost:8080/api/v1/health",
      reuseExistingServer: false, // Always start fresh so the server uses this config's settings
      timeout: 30000,
      // Playwright kills the process group with SIGKILL by default, which skips
      // run-dev.py's cleanup and leaves its temporary config directory behind.
      gracefulShutdown: { signal: "SIGTERM", timeout: 5000 },
    },
  };
}

export default defineConfig(playwrightConfig(true));
