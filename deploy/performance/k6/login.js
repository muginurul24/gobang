import http from 'k6/http';
import { check } from 'k6';

import { baseURL, jsonHeaders, ownerLogin, ownerPassword } from './lib/config.js';
import { safeJSON } from './lib/helpers.js';

export let options = {
  vus: parseInt(__ENV.K6_LOGIN_VUS || '10', 10),
  duration: __ENV.K6_DURATION || '20s',
  summaryTrendStats: ['avg', 'med', 'p(95)', 'p(99)', 'max']
};

export default function () {
  const response = http.post(
    `${baseURL}/v1/auth/login`,
    JSON.stringify({
      login: ownerLogin,
      password: ownerPassword
    }),
    {
      headers: jsonHeaders(),
      tags: { flow: 'auth_login' }
    }
  );

  const payload = safeJSON(response);
  check(response, {
    'login http 200': (res) => res.status === 200,
    'login status success': () => payload && payload.status === true,
    'login access token present': () => payload && payload.data && payload.data.access_token !== ''
  });
}
