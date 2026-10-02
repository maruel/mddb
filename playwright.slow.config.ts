// Playwright configuration for the slow e2e suite: production rate limits.
import { defineConfig } from "@playwright/test";
import { playwrightConfig } from "./playwright.config";

// The server starts without the e2e-only -fast-rate-limit flag, so the suite
// observes the rate limits a production deployment enforces.
export default defineConfig(playwrightConfig(false));
