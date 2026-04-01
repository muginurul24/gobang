import { browser } from '$app/environment';
import { writable } from 'svelte/store';

export type ThemePreference = 'system' | 'light' | 'dark';
export type ResolvedTheme = 'light' | 'dark';

const storageKey = 'onixggr.theme.preference';

export const themePreference = writable<ThemePreference>('system');
export const resolvedTheme = writable<ResolvedTheme>('dark');

let initialized = false;
let mediaQuery: MediaQueryList | null = null;

export function ensureThemeHydrated() {
  if (!browser || initialized) {
    return;
  }

  initialized = true;
  mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');

  const stored = readStoredPreference();
  applyTheme(stored);
  themePreference.set(stored);

  mediaQuery.addEventListener('change', () => {
    const currentPreference = readStoredPreference();
    if (currentPreference === 'system') {
      applyTheme(currentPreference);
    }
  });
}

export function setTheme(next: ThemePreference) {
  if (!browser) {
    return;
  }

  window.localStorage.setItem(storageKey, next);
  themePreference.set(next);
  applyTheme(next);
}

function readStoredPreference(): ThemePreference {
  if (!browser) {
    return 'system';
  }

  const raw = window.localStorage.getItem(storageKey);
  return raw === 'light' || raw === 'dark' || raw === 'system' ? raw : 'system';
}

function resolveTheme(preference: ThemePreference): ResolvedTheme {
  if (preference === 'light' || preference === 'dark') {
    return preference;
  }

  return mediaQuery?.matches ? 'dark' : 'light';
}

function applyTheme(preference: ThemePreference) {
  if (!browser) {
    return;
  }

  const nextResolved = resolveTheme(preference);
  const root = document.documentElement;
  root.dataset.themePreference = preference;
  root.dataset.theme = nextResolved;
  resolvedTheme.set(nextResolved);
}
