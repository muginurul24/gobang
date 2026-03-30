import exec from 'k6/execution';

export function uniqueRef(prefix) {
  return `${prefix}-${exec.vu.idInTest}-${exec.vu.iterationInScenario}-${Date.now()}`;
}

export function safeJSON(response) {
  try {
    return response.json();
  } catch (_) {
    return null;
  }
}
