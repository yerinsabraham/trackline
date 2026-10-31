import { defineConfig, devices } from "@playwright/test";

// WebKit with an iPhone's screen, because the person this app is for reads it
// on an iPhone, and Chrome draws things WebKit does not. Chromium runs too, so
// a failure says which engine it belongs to. Needs `npm run build` first.
export default defineConfig({
  testDir: "e2e",
  fullyParallel: true,
  // WebKit is slow to start, and more of them at once fail by timing out.
  workers: 3,
  reporter: process.env.CI ? "github" : "list",
  use: { baseURL: "http://localhost:4173", trace: "off" },
  webServer: { command: "node e2e/serve.mjs", url: "http://localhost:4173/app", reuseExistingServer: !process.env.CI },
  projects: [
    { name: "iphone-webkit", use: { ...devices["iPhone 13"] } },
    { name: "phone-chromium", use: { ...devices["Pixel 7"] } },
  ],
});
