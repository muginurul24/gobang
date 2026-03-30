import { check } from 'k6';
import exec from 'k6/execution';
import ws from 'k6/ws';
import { Counter, Trend } from 'k6/metrics';

import { baseURL, wsHoldDuration } from './lib/config.js';
import { loginOwner } from './lib/auth.js';

const wsConnectDuration = new Trend('onixggr_ws_connect_duration');
const wsSessionDuration = new Trend('onixggr_ws_session_duration');
const wsHelloTotal = new Counter('onixggr_ws_hello_total');
const wsPongTotal = new Counter('onixggr_ws_pong_total');
const wsHeartbeatTotal = new Counter('onixggr_ws_heartbeat_total');

export let options = {
  vus: parseInt(__ENV.K6_WS_VUS || '50', 10),
  iterations: parseInt(__ENV.K6_WS_ITERATIONS || (__ENV.K6_WS_VUS || '50'), 10),
  summaryTrendStats: ['avg', 'med', 'p(95)', 'p(99)', 'max']
};

export function setup() {
  return loginOwner();
}

export default function (session) {
  const startedAt = Date.now();
  const url = baseURL.replace(/^http/, 'ws') + `/v1/realtime/ws?access_token=${encodeURIComponent(session.accessToken)}`;
  const holdMs = durationToMs(wsHoldDuration);

  const response = ws.connect(url, {}, function (socket) {
    let pingSent = false;

    socket.on('open', function () {
      wsConnectDuration.add(Date.now() - startedAt);
    });

    socket.on('message', function (message) {
      let frame;
      try {
        frame = JSON.parse(message);
      } catch (_) {
        return;
      }

      if (frame.kind === 'hello') {
        wsHelloTotal.add(1);
        if (!pingSent) {
          pingSent = true;
          socket.send(JSON.stringify({ type: 'ping', vu: exec.vu.idInTest }));
        }
      }

      if (frame.kind === 'heartbeat') {
        wsHeartbeatTotal.add(1);
      }

      if (frame.kind === 'event' && frame.event && frame.event.type === 'realtime.pong') {
        wsPongTotal.add(1);
      }
    });

    socket.setTimeout(function () {
      socket.close();
    }, holdMs);

    socket.on('close', function () {
      wsSessionDuration.add(Date.now() - startedAt);
    });
  });

  check(response, {
    'websocket upgrade 101': (res) => res && res.status === 101
  });
}

function durationToMs(raw) {
  const trimmed = String(raw || '').trim();
  if (trimmed.endsWith('ms')) {
    return Number(trimmed.slice(0, -2));
  }
  if (trimmed.endsWith('s')) {
    return Number(trimmed.slice(0, -1)) * 1000;
  }
  if (trimmed.endsWith('m')) {
    return Number(trimmed.slice(0, -1)) * 60 * 1000;
  }

  return 10000;
}
