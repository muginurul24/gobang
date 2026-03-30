import { writable } from 'svelte/store';

import type { Store } from '$lib/stores/client';

const storageKey = 'onixggr.preferred_store_id';

export const preferredStoreID = writable('');

export function hydratePreferredStoreID() {
  if (typeof window === 'undefined') {
    return '';
  }

  const value = window.localStorage.getItem(storageKey)?.trim() ?? '';
  preferredStoreID.set(value);
  return value;
}

export function setPreferredStoreID(storeID: string) {
  const value = storeID.trim();
  preferredStoreID.set(value);

  if (typeof window === 'undefined') {
    return;
  }

  if (value === '') {
    window.localStorage.removeItem(storageKey);
    return;
  }

  window.localStorage.setItem(storageKey, value);
}

export function pickPreferredStoreID(stores: Store[], currentStoreID: string) {
  const current = currentStoreID.trim();
  if (current !== '' && stores.some((store) => store.id === current)) {
    return current;
  }

  let preferred = '';
  const unsubscribe = preferredStoreID.subscribe((value) => {
    preferred = value;
  });
  unsubscribe();

  if (preferred !== '' && stores.some((store) => store.id === preferred)) {
    return preferred;
  }

  return stores[0]?.id ?? '';
}
