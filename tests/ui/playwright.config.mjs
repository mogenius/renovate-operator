import { defineConfig, devices } from "@playwright/test";

const staticFrontendPort = Number(process.env.STATIC_FRONTEND_PORT || 8099);
const staticFrontendUrl = `http://127.0.0.1:${staticFrontendPort}`;

export default defineConfig({
  testDir: "./specs",
  // The page ships untranspiled JSX and Babel compiles it in the browser on every
  // load, so first paint is slower than a bundled app would be.
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["github"], ["html", { open: "never" }]] : [["list"]],
  use: {
    baseURL: staticFrontendUrl,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    {
      name: "chromium",
      // A fixed viewport keeps the above-the-fold assertions in
      // jobCardExpansion.spec.mjs meaningful across machines.
      use: { ...devices["Desktop Chrome"], viewport: { width: 1280, height: 900 } },
    },
  ],
  webServer: {
    command: "node staticFrontendServer.mjs",
    url: staticFrontendUrl,
    // Never reuse a server when a page is overridden: a leftover server from a
    // previous run would silently serve the wrong revision of it.
    reuseExistingServer:
      !process.env.CI && !process.env.INDEX_HTML_PATH && !process.env.LOGS_HTML_PATH,
    stdout: "ignore",
    stderr: "pipe",
    env: {
      STATIC_FRONTEND_PORT: String(staticFrontendPort),
      BASE_PATH: process.env.BASE_PATH || "",
      INDEX_HTML_PATH: process.env.INDEX_HTML_PATH || "",
      LOGS_HTML_PATH: process.env.LOGS_HTML_PATH || "",
      // Specs stub the API themselves; pinned off so an exported MOCK_API from a
      // `just ui-dev` shell cannot change what the suite runs against.
      MOCK_API: "",
    },
  },
});
