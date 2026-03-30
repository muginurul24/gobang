import http from 'k6/http';
import { check } from 'k6';

import { baseURL, jsonHeaders, memberUsername, storeToken } from './lib/config.js';
import { safeJSON, uniqueRef } from './lib/helpers.js';

export let options = {
  vus: parseInt(__ENV.K6_GAME_DEPOSIT_VUS || '2', 10),
  duration: __ENV.K6_DURATION || '20s',
  summaryTrendStats: ['avg', 'med', 'p(95)', 'p(99)', 'max']
};

export default function () {
  const response = http.post(
    `${baseURL}/v1/store-api/game/deposits`,
    JSON.stringify({
      username: memberUsername,
      amount: 5000,
      trx_id: uniqueRef('perf-deposit')
    }),
    {
      headers: jsonHeaders({
        Authorization: `Bearer ${storeToken}`
      }),
      tags: { flow: 'game_deposit' }
    }
  );

  const payload = safeJSON(response);
  check(response, {
    'deposit 201 or 202': (res) => res.status === 201 || res.status === 202,
    'deposit success flag': () => payload && payload.status === true,
    'deposit success or pending': () => payload && (payload.message === 'SUCCESS' || payload.message === 'PENDING_RECONCILE')
  });
}
