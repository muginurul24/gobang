import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';

import { chromium, devices } from '@playwright/test';

const baseURL = requiredEnv('BASE_URL');
const loginEmail = requiredEnv('LOGIN_EMAIL');
const loginPassword = requiredEnv('LOGIN_PASSWORD');
const outputDir = process.env.OUT_DIR || '.tmp/frontend-pass';
const chromiumPath = process.env.CHROMIUM_PATH || '/usr/bin/chromium-browser';
const themes = (process.env.THEMES || 'dark,light')
  .split(',')
  .map((value) => value.trim())
  .filter(
    (value) => value === 'dark' || value === 'light' || value === 'system',
  );

const publicRoutes = ['/', '/login'];

const desktopAppRoutes = [
  '/app',
  '/app/onboarding',
  '/app/users',
  '/app/ops',
  '/app/stores',
  '/app/api-docs',
  '/app/catalog',
  '/app/members',
  '/app/topups',
  '/app/bank-accounts',
  '/app/withdrawals',
  '/app/notifications',
  '/app/security',
  '/app/chat',
  '/app/audit',
];

const mobileRoutes = [
  '/app',
  '/app/onboarding',
  '/app/users',
  '/app/ops',
  '/app/stores',
  '/app/topups',
  '/app/withdrawals',
  '/app/notifications',
  '/app/chat',
];

await mkdir(outputDir, { recursive: true });

const browser = await chromium.launch({
  executablePath: chromiumPath,
  headless: true,
});

try {
  const captures = [];
  for (const theme of themes) {
    const publicContext = await browser.newContext({
      viewport: { width: 1512, height: 982 },
      colorScheme: colorSchemeFor(theme),
    });
    const publicPage = await publicContext.newPage();
    await applyThemePreference(publicPage, theme);

    for (const route of publicRoutes) {
      const file = path.join(
        outputDir,
        `desktop-${theme}${routeToFilename(route)}.png`,
      );
      await captureRoute(publicPage, route, file);
      captures.push({ route, viewport: 'desktop', theme, file });
    }

    await publicPage.close().catch(() => null);
    await publicContext.close();

    const desktopContext = await browser.newContext({
      viewport: { width: 1512, height: 982 },
      colorScheme: colorSchemeFor(theme),
    });
    const desktopPage = await desktopContext.newPage();
    await applyThemePreference(desktopPage, theme);
    const session = await loginThroughUI(desktopPage);

    for (const route of desktopAppRoutes) {
      const file = path.join(
        outputDir,
        `desktop-${theme}${routeToFilename(route)}.png`,
      );
      await captureRoute(desktopPage, route, file, session);
      captures.push({ route, viewport: 'desktop', theme, file });
    }

    await desktopPage.close().catch(() => null);
    await desktopContext.close();

    const mobileContext = await browser.newContext({
      ...devices['iPhone 13'],
      colorScheme: colorSchemeFor(theme),
    });
    const mobilePage = await mobileContext.newPage();
    await applyThemePreference(mobilePage, theme);
    const mobileSession = await loginThroughUI(mobilePage);
    await captureRoute(
      mobilePage,
      '/app',
      path.join(outputDir, `mobile-${theme}${routeToFilename('/app')}.png`),
      mobileSession,
    );
    captures.push({
      route: '/app',
      viewport: 'mobile',
      theme,
      file: path.join(
        outputDir,
        `mobile-${theme}${routeToFilename('/app')}.png`,
      ),
    });

    for (const route of mobileRoutes.slice(1)) {
      const file = path.join(
        outputDir,
        `mobile-${theme}${routeToFilename(route)}.png`,
      );
      await captureRoute(mobilePage, route, file, mobileSession);
      captures.push({ route, viewport: 'mobile', theme, file });
    }

    await mobilePage.close().catch(() => null);
    await mobileContext.close();
  }

  await writeFile(
    path.join(outputDir, 'manifest.json'),
    JSON.stringify(
      {
        generated_at: new Date().toISOString(),
        base_url: baseURL,
        captures,
      },
      null,
      2,
    ),
  );
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

      const rawBody = await result.text();
      let parsed = null;

      try {
        parsed = rawBody === '' ? null : JSON.parse(rawBody);
      } catch {
        parsed = null;
      }

      return {
        ok: result.ok,
        status: result.status,
        contentType: result.headers.get('content-type') || '',
        body: parsed,
        rawBody,
      };
    },
    {
      email: loginEmail,
      password: loginPassword,
    },
  );

  if (!response?.body?.status || response?.body?.message !== 'SUCCESS') {
    throw new Error(
      `Local login bootstrap failed: ${JSON.stringify({
        http_status: response?.status,
        content_type: response?.contentType,
        body: response?.body,
        raw_body: response?.rawBody?.slice(0, 500),
      })}`,
    );
  }

  await page.evaluate((session) => {
    window.sessionStorage.setItem(
      'onixggr.dashboard.auth',
      JSON.stringify(session),
    );
  }, response.body.data);

  await page.goto(resolveURL('/app'), { waitUntil: 'domcontentloaded' });
  await waitForAppReady(page, '/app', response.body.data);
  return response.body.data;
}

async function captureRoute(page, route, file, session = null) {
  const url = resolveURL(route);

  try {
    await page.goto(url, { waitUntil: 'domcontentloaded' });
  } catch (error) {
    if (!String(error).includes('ERR_ABORTED')) {
      throw error;
    }

    await page.waitForTimeout(800);
    await page.goto(url, { waitUntil: 'domcontentloaded' });
  }

  if (route.startsWith('/app')) {
    await waitForAppReady(page, route, session);
  } else {
    await page.waitForTimeout(1000);
  }
  await page.screenshot({
    path: file,
    fullPage: true,
  });
}

async function waitForAppReady(page, route, session) {
  for (let attempt = 0; attempt < 3; attempt += 1) {
    await page.waitForTimeout(1200);

    if (page.url().includes('/login')) {
      if (!session) {
        return;
      }

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
      const routeSettled = await page
        .waitForFunction(() => {
          const titleReady = document.title.trim().length > 0;
          const activeNavCount = document.querySelectorAll(
            '.app-nav-link[data-active="true"]',
          ).length;
          return titleReady && activeNavCount >= 1;
        })
        .then(() => true)
        .catch(() => false);

      if (routeSettled) {
        await page.waitForTimeout(700);
        return;
      }
    }

    const shellHeading = page.locator('text=Memeriksa sesi dashboard...');
    const shellVisible = await shellHeading.isVisible().catch(() => false);
    if (!shellVisible) {
      await page.waitForTimeout(900);
      return;
    }

    await readyShell
      .first()
      .waitFor({ state: 'visible', timeout: 6000 })
      .catch(() => null);
    const readyAfterWait = await readyShell
      .first()
      .isVisible()
      .catch(() => false);
    if (readyAfterWait) {
      const routeSettled = await page
        .waitForFunction(() => {
          const titleReady = document.title.trim().length > 0;
          const activeNavCount = document.querySelectorAll(
            '.app-nav-link[data-active="true"]',
          ).length;
          return titleReady && activeNavCount >= 1;
        })
        .then(() => true)
        .catch(() => false);

      if (routeSettled) {
        await page.waitForTimeout(700);
        return;
      }
    }
  }

  await page.waitForTimeout(1000);
}

async function applyThemePreference(page, theme) {
  await page.addInitScript((nextTheme) => {
    const storageKey = 'onixggr.theme.preference';
    window.localStorage.setItem(storageKey, nextTheme);
    const resolved =
      nextTheme === 'system'
        ? window.matchMedia('(prefers-color-scheme: dark)').matches
          ? 'dark'
          : 'light'
        : nextTheme;
    document.documentElement.dataset.themePreference = nextTheme;
    document.documentElement.dataset.theme = resolved;
  }, theme);
}

function colorSchemeFor(theme) {
  return theme === 'light' ? 'light' : 'dark';
}

function resolveURL(route) {
  return `${baseURL.replace(/\/$/, '')}${route}`;
}

function routeToFilename(route) {
  if (route === '/') {
    return '-home';
  }

  return `-${route.replaceAll('/', '-').replace(/^-+/, '')}`;
}

function requiredEnv(name) {
  const value = process.env[name]?.trim();
  if (!value) {
    throw new Error(`Missing required env: ${name}`);
  }

  return value;
}
