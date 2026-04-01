import { chromium } from '@playwright/test';

const baseURL = requiredEnv('BASE_URL');
const loginValue = requiredEnv('LOGIN_EMAIL');
const loginPassword = requiredEnv('LOGIN_PASSWORD');
const chromiumPath = process.env.CHROMIUM_PATH || '/usr/bin/chromium-browser';

const routes = [
  '/app',
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

const browser = await chromium.launch({
  executablePath: chromiumPath,
  headless: true,
});

try {
  const context = await browser.newContext({
    viewport: { width: 1512, height: 982 },
    colorScheme: 'dark',
  });
  const page = await context.newPage();
  const networkLog = [];

  page.on('response', async (response) => {
    if (!response.url().includes('/v1/')) {
      return;
    }

    networkLog.push({
      url: response.url(),
      status: response.status(),
      method: response.request().method(),
    });
  });

  await page.goto(resolveURL('/login'), { waitUntil: 'domcontentloaded' });
  const loginResponse = await page.evaluate(
    async ({ email, password }) => {
      const response = await fetch('/v1/auth/login', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          login: email,
          password,
        }),
      });

      let body = null;
      try {
        body = await response.json();
      } catch {
        body = null;
      }

      if (body?.status && body?.message === 'SUCCESS') {
        window.sessionStorage.setItem(
          'onixggr.dashboard.auth',
          JSON.stringify(body.data),
        );
      }

      return {
        status: response.status,
        body,
        cookie: document.cookie,
        session: window.sessionStorage.getItem('onixggr.dashboard.auth'),
      };
    },
    { email: loginValue, password: loginPassword },
  );

  await page.goto(resolveURL('/app'), { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1500);

  const loginSnapshot = await collectSnapshot(page, context, 'after-login');
  loginSnapshot.login_response = {
    status: loginResponse.status,
    body: loginResponse.body,
    login_cookie_string: loginResponse.cookie,
    login_session_present: loginResponse.session !== null,
  };
  loginSnapshot.network = networkLog.splice(0);
  const results = [loginSnapshot];

  for (const route of routes) {
    await page.goto(resolveURL(route), { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(1500);
    const snapshot = await collectSnapshot(page, context, route);
    snapshot.network = networkLog.splice(0);
    results.push(snapshot);
  }

  console.log(JSON.stringify(results, null, 2));
} finally {
  await browser.close();
}

async function collectSnapshot(page, context, label) {
  const cookies = await context.cookies();
  const authCookies = cookies.filter((cookie) =>
    ['onixggr_refresh_token', 'onixggr_csrf_token'].includes(cookie.name),
  );
  const authStorage = await page.evaluate(() => ({
    session: window.sessionStorage.getItem('onixggr.dashboard.auth'),
    theme: window.localStorage.getItem('onixggr.theme.preference'),
    cookie: document.cookie,
  }));
  const layoutState = await page.evaluate(() => {
    const doc = document.documentElement;
    const body = document.body;
    const overflowThreshold = 1;
    const rootScrollWidth = Math.max(
      doc?.scrollWidth ?? 0,
      body?.scrollWidth ?? 0,
    );
    const rootClientWidth = Math.max(
      doc?.clientWidth ?? 0,
      body?.clientWidth ?? 0,
    );
    const activeLinks = Array.from(
      document.querySelectorAll('.app-nav-link[data-active="true"]'),
    );
    const pageHeading =
      document.querySelector('#app-main h1')?.textContent?.trim() ?? '';
    const offenders = Array.from(
      document.querySelectorAll('main *, section *, article *, aside *'),
    )
      .map((element) => {
        const node = /** @type {HTMLElement} */ (element);
        return {
          tag: node.tagName.toLowerCase(),
          classes: Array.from(node.classList).slice(0, 4),
          scrollWidth: node.scrollWidth,
          clientWidth: node.clientWidth,
        };
      })
      .filter(
        (item) =>
          item.scrollWidth > 0 &&
          item.clientWidth > 0 &&
          item.scrollWidth - item.clientWidth > overflowThreshold,
      )
      .sort((left, right) => right.scrollWidth - left.scrollWidth)
      .slice(0, 5);

    return {
      active_nav_count: activeLinks.length,
      active_nav_labels: activeLinks
        .map((link) => link.querySelector('.app-nav-link__label')?.textContent?.trim() ?? '')
        .filter(Boolean),
      page_heading: pageHeading,
      root_scroll_width: rootScrollWidth,
      root_client_width: rootClientWidth,
      has_horizontal_overflow:
        rootScrollWidth - rootClientWidth > overflowThreshold,
      overflow_offenders: offenders,
    };
  });

  return {
    label,
    url: page.url(),
    title: await page.title(),
    auth_cookies: authCookies.map((cookie) => ({
      name: cookie.name,
      domain: cookie.domain,
      path: cookie.path,
      httpOnly: cookie.httpOnly,
      secure: cookie.secure,
      sameSite: cookie.sameSite,
    })),
    has_session_storage: authStorage.session !== null,
    cookie_string: authStorage.cookie,
    theme: authStorage.theme,
    layout: layoutState,
  };
}

function resolveURL(path) {
  return new URL(path, baseURL).toString();
}

function requiredEnv(name) {
  const value = process.env[name];
  if (!value) {
    throw new Error(`Missing required environment variable: ${name}`);
  }

  return value;
}
