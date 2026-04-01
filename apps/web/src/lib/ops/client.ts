export type HealthDependency = {
  name: string;
  status: 'ok' | 'degraded' | 'error';
  error?: string;
};

export type HealthReport = {
  status: 'ok' | 'ready' | 'degraded' | 'not_ready';
  service: string;
  environment: string;
  timestamp: string;
  dependencies?: HealthDependency[];
};

export async function fetchLiveHealth() {
  return fetchHealth('/health/live');
}

export async function fetchReadyHealth() {
  return fetchHealth('/health/ready');
}

async function fetchHealth(path: string) {
  const response = await fetch(path, {
    credentials: 'include',
    headers: {
      Accept: 'application/json',
    },
  });

  if (!response.ok) {
    throw new Error(`health request failed: ${response.status}`);
  }

  return (await response.json()) as HealthReport;
}
