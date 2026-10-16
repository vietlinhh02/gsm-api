import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/browser",
  fullyParallel: false,
  timeout: 45000,
  use: {
    baseURL: "http://localhost:3000",
    viewport: { width: 1440, height: 1000 },
    launchOptions: { executablePath: "/usr/bin/google-chrome", args: ["--no-sandbox"] },
    trace: "retain-on-failure",
  },
  webServer: {
    command: "pnpm dev --hostname 127.0.0.1",
    url: "http://localhost:3000",
    reuseExistingServer: true,
  },
});
