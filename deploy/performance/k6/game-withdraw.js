import http from 'k6/http';
import { check } from 'k6';

import { baseURL, jsonHeaders, memberUsername, storeToken } from './lib/config.js';
import { safeJSON, uniqueRef } from './lib/helpers.js';

export let options = {
  vus: parseInt(__ENV.K6_GAME_WITHDRAW_VUS || '2', 10),
  duration: __ENV.K6_DURATION || '20s',
  summaryTrendStats: ['avg', 'med', 'p(95)', 'p(99)', 'max']
};

export default function () {
  const response = http.post(
    `${baseURL}/v1/store-api/game/withdrawals`,
    JSON.stringify({
      username: memberUsername,
      amount: 5000,
      trx_id: uniqueRef('perf-withdraw')
    }),
    {
      headers: jsonHeaders({
        Authorization: `Bearer ${storeToken}`
      }),
      tags: { flow: 'game_withdraw' }
    }
  );

  const payload = safeJSON(response);
  check(response, {
    'withdraw 201 or 202': (res) => res.status === 201 || res.status === 202,
    'withdraw success flag': () => payload && payload.status === true,
    'withdraw success or pending': () => payload && (payload.message === 'SUCCESS' || payload.message === 'PENDING_RECONCILE')
  });
}
