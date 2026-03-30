import http from 'k6/http';
import { check } from 'k6';

import { baseURL, jsonHeaders, storeID } from './lib/config.js';
import { loginOwner } from './lib/auth.js';
import { safeJSON } from './lib/helpers.js';

export let options = {
  vus: parseInt(__ENV.K6_QRIS_GENERATE_VUS || '6', 10),
  duration: __ENV.K6_DURATION || '20s',
  summaryTrendStats: ['avg', 'med', 'p(95)', 'p(99)', 'max']
};

export function setup() {
  return loginOwner();
}

export default function (session) {
  const response = http.post(
    `${baseURL}/v1/stores/${storeID}/topups/qris`,
    JSON.stringify({ amount: 10000 }),
    {
      headers: jsonHeaders({
        Authorization: `Bearer ${session.accessToken}`,
        'X-CSRF-Token': session.csrfToken,
        Cookie: `onixggr_csrf_token=${session.csrfToken}`
      }),
      tags: { flow: 'qris_generate' }
    }
  );

  const payload = safeJSON(response);
  check(response, {
    'qris generate 201 or 202': (res) => res.status === 201 || res.status === 202,
    'qris generate success flag': () => payload && payload.status === true,
    'qris generate success or pending': () => payload && (payload.message === 'SUCCESS' || payload.message === 'PENDING_PROVIDER')
  });
}
