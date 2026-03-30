import http from 'k6/http';
import { check, fail } from 'k6';

import { altMemberUsername, baseURL, jsonHeaders, storeToken } from './lib/config.js';
import { safeJSON } from './lib/helpers.js';

export let options = {
  vus: parseInt(__ENV.K6_WEBHOOK_VUS || '24', 10),
  duration: __ENV.K6_WEBHOOK_DURATION || '15s',
  summaryTrendStats: ['avg', 'med', 'p(95)', 'p(99)', 'max']
};

export function setup() {
  const paymentResponse = http.post(
    `${baseURL}/v1/store-api/qris/member-payments`,
    JSON.stringify({
      username: altMemberUsername,
      amount: 15000
    }),
    {
      headers: jsonHeaders({
        Authorization: `Bearer ${storeToken}`
      }),
      tags: { flow: 'webhook_seed' }
    }
  );

  const payload = safeJSON(paymentResponse);
  if (paymentResponse.status !== 201 || !payload || payload.status !== true) {
    fail(`failed to seed member payment: ${paymentResponse.status} ${paymentResponse.body}`);
  }

  const transaction = payload.data;
  return {
    amount: Number(transaction.amount_gross),
    customRef: transaction.custom_ref,
    trxID: transaction.provider_trx_id
  };
}

export default function (transaction) {
  const response = http.post(
    `${baseURL}/v1/webhooks/qris`,
    JSON.stringify({
      amount: transaction.amount,
      terminal_id: altMemberUsername,
      trx_id: transaction.trxID,
      rrn: 'mock-rrn-duplicate',
      custom_ref: transaction.customRef,
      vendor: 'mock-qris',
      status: 'success',
      create_at: '2026-03-31T12:00:00Z',
      finish_at: '2026-03-31T12:00:02Z'
    }),
    {
      headers: jsonHeaders(),
      tags: { flow: 'webhook_burst' }
    }
  );

  const payload = safeJSON(response);
  check(response, {
    'webhook burst http 200': (res) => res.status === 200,
    'webhook burst handled': () => payload && (payload.message === 'SUCCESS' || payload.message === 'IGNORED')
  });
}
