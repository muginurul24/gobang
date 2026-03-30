export const baseURL = env('K6_BASE_URL', 'http://127.0.0.1:18080');
export const storeID = env('K6_STORE_ID', 'cccccccc-cccc-cccc-cccc-cccccccccccc');
export const storeToken = env('K6_STORE_TOKEN', 'store_live_demo');
export const ownerLogin = env('K6_OWNER_LOGIN', 'owner@example.com');
export const ownerPassword = env('K6_OWNER_PASSWORD', 'OwnerDemo123!');
export const memberUsername = env('K6_MEMBER_USERNAME', 'member-demo');
export const altMemberUsername = env('K6_ALT_MEMBER_USERNAME', 'member-alpha');
export const runDuration = env('K6_DURATION', '20s');
export const webhookBurstDuration = env('K6_WEBHOOK_DURATION', '15s');
export const wsHoldDuration = env('K6_WS_HOLD', '10s');
export const trendStats = ['avg', 'med', 'p(95)', 'p(99)', 'max'];

export function env(name, fallback) {
  const value = __ENV[name];
  if (value === undefined || value === '') {
    return fallback;
  }

  return value;
}

export function jsonHeaders(extra = {}) {
  const headers = {
    'content-type': 'application/json',
    accept: 'application/json'
  };

  for (const key in extra) {
    if (Object.prototype.hasOwnProperty.call(extra, key)) {
      headers[key] = extra[key];
    }
  }

  return headers;
}
