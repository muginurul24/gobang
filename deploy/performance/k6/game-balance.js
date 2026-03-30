import http from 'k6/http';
import { check } from 'k6';

import { baseURL, memberUsername, storeToken } from './lib/config.js';
import { safeJSON } from './lib/helpers.js';

export let options = {
  vus: parseInt(__ENV.K6_GAME_BALANCE_VUS || '20', 10),
  duration: __ENV.K6_DURATION || '20s',
  summaryTrendStats: ['avg', 'med', 'p(95)', 'p(99)', 'max']
};

export default function () {
  const response = http.get(
    `${baseURL}/v1/store-api/game/balance?username=${encodeURIComponent(memberUsername)}`,
    {
      headers: {
        Authorization: `Bearer ${storeToken}`,
        accept: 'application/json'
      },
      tags: { flow: 'game_balance' }
    }
  );

  const payload = safeJSON(response);
  check(response, {
    'balance http 200': (res) => res.status === 200,
    'balance success': () => payload && payload.status === true,
    'balance payload present': () => payload && payload.data && payload.data.balance !== ''
  });
}
