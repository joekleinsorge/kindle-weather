const { defineConfig } = require('@playwright/test');

// CSS viewport sizes, not physical pixels. Reduced heights reserve browser chrome.
const screens = [
  ['kindle-600-full', 600, 800],
  ['kindle-600-toolbar', 600, 660],
  ['paperwhite-full', 758, 1024],
  ['paperwhite-toolbar', 758, 880],
  ['landscape-full', 1024, 758],
  ['landscape-toolbar', 1024, 600],
];
module.exports = defineConfig({
  testDir: './tests/layout',
  workers: 2,
  use: { baseURL: 'http://127.0.0.1:18769', screenshot: 'only-on-failure', trace: 'retain-on-failure' },
  projects: ['chromium', 'webkit'].flatMap(browserName => screens.map(([name, width, height]) => ({
    name: `${browserName}-${name}`, use: { browserName, viewport: { width, height } },
  }))),
  webServer: {
    command: 'go test -run ^TestLayoutFixtureServer$ -count=1 -timeout=0',
    env: { KINDLE_LAYOUT_SERVER: '1' },
    url: 'http://127.0.0.1:18769',
    timeout: 120000,
  },
});
