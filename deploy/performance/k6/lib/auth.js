import http from 'k6/http';
import { check, fail } from 'k6';

import { baseURL, jsonHeaders, ownerLogin, ownerPassword } from './config.js';

export function loginOwner() {
  const response = http.post(
    `${baseURL}/v1/auth/login`,
    JSON.stringify({
      login: ownerLogin,
      password: ownerPassword
    }),
    {
      headers: jsonHeaders(),
      tags: { flow: 'auth_login_setup' }
    }
  );

  const ok = check(response, {
    'login setup status 200': (res) => res.status === 200,
    'login setup success': (res) => {
      const payload = safeJSON(res);
      return payload && payload.status === true;
    },
    'login setup access token': (res) => {
      const payload = safeJSON(res);
      return payload && payload.data && payload.data.access_token !== '';
    }
  });
  if (!ok) {
    fail(`owner login failed: ${response.status} ${response.body}`);
  }

  const payload = safeJSON(response);
  const csrfCookie = firstCookieValue(response.cookies, 'onixggr_csrf_token');
  if (csrfCookie === '') {
    fail(`csrf cookie missing: ${response.body}`);
  }

  return {
    accessToken: payload.data.access_token,
    csrfToken: csrfCookie
  };
}

function safeJSON(response) {
  try {
    return response.json();
  } catch (_) {
    return null;
  }
}

function firstCookieValue(cookies, name) {
  if (!cookies || !cookies[name] || !cookies[name][0]) {
    return '';
  }

  return cookies[name][0].value || '';
}
