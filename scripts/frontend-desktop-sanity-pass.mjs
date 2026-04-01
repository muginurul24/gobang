import { mkdir } from 'node:fs/promises';
import path from 'node:path';

import { chromium } from '@playwright/test';

const baseURL = requiredEnv('BASE_URL');
const loginEmail = requiredEnv('LOGIN_EMAIL');
const loginPassword = requiredEnv('LOGIN_PASSWORD');
const outputDir = process.env.OUT_DIR || '.tmp/frontend-desktop-sanity';
const chromiumPath = process.env.CHROMIUM_PATH || '/usr/bin/chromium-browser';
const theme = (process.env.THEME || 'dark').trim();
const routes = (
  process.env.ROUTES ||
  '/app,/app/users,/app/stores,/app/notifications,/app/ops,/app/security'
)
  .split(',')
  .map((route) => route.trim())
  .filter(Boolean);

await mkdir(outputDir, { recursive: true });

const browser = await chromium.launch({
  executablePath: chromiumPath,
  headless: true,
});

try {
  const context = await browser.newContext({
    viewport: { width: 1512, height: 982 },
    colorScheme: theme === 'light' ? 'light' : 'dark',
  });
  const page = await context.newPage();

  await applyThemePreference(page, theme);
  const session = await loginThroughUI(page);

  for (const route of routes) {
    await page.goto(resolveURL(route), { waitUntil: 'domcontentloaded' });
    await waitForAppReady(page, route, session);
    await page.screenshot({
      path: path.join(
        outputDir,
        `${theme}-${route.replaceAll('/', '-').replace(/^-+/, '')}.png`,
      ),
      fullPage: true,
    });
  }
} finally {
  await browser.close();
}

async function loginThroughUI(page) {
  await page.goto(resolveURL('/login'), { waitUntil: 'domcontentloaded' });
  const response = await page.evaluate(
    async ({ email, password }) => {
      const result = await fetch('/v1/auth/login', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          login: email,
          password,
        }),
      });

      const body = await result.json();
      if (body?.status && body?.message === 'SUCCESS') {
        window.sessionStorage.setItem(
          'onixggr.dashboard.auth',
          JSON.stringify(body.data),
        );
      }

      return body?.data ?? null;
    },
    {
      email: loginEmail,
      password: loginPassword,
    },
  );

  if (!response) {
    throw new Error('Login bootstrap failed');
  }

  await page.goto(resolveURL('/app'), { waitUntil: 'domcontentloaded' });
  await waitForAppReady(page, '/app', response);
  return response;
}

async function waitForAppReady(page, route, session) {
  for (let attempt = 0; attempt < 3; attempt += 1) {
    await page.waitForTimeout(1200);

    if (page.url().includes('/login')) {
      await page.evaluate((authSession) => {
        window.sessionStorage.setItem(
          'onixggr.dashboard.auth',
          JSON.stringify(authSession),
        );
      }, session);
      await page.goto(resolveURL(route), { waitUntil: 'domcontentloaded' });
      continue;
    }

    const readyShell = page.locator('[data-app-shell="ready"]');
    const readyVisible = await readyShell
      .first()
      .isVisible()
      .catch(() => false);
    if (readyVisible) {
      await page.waitForTimeout(900);
      return;
    }

    await page.waitForTimeout(1000);
  }
}

async function applyThemePreference(page, nextTheme) {
  await page.addInitScript((initialTheme) => {
    const storageKey = 'onixggr.theme.preference';
    window.localStorage.setItem(storageKey, initialTheme);
    document.documentElement.dataset.themePreference = initialTheme;
    document.documentElement.dataset.theme = initialTheme;
  }, nextTheme);
}

function resolveURL(route) {
  return `${baseURL.replace(/\/$/, '')}${route}`;
}

function requiredEnv(name) {
  const value = process.env[name]?.trim();
  if (!value) {
    throw new Error(`Missing required env: ${name}`);
  }

  return value;
}
