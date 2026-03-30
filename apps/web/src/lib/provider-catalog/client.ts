import { apiRequest } from '$lib/auth/client';

export type CatalogProvider = {
  provider_code: string;
  provider_name: string;
  status: number;
  synced_at: string;
};

export type CatalogGame = {
  provider_code: string;
  game_code: string;
  game_name: string;
  banner_url: string;
  status: number;
  synced_at: string;
};

export async function fetchCatalogProviders(
  params: {
    query?: string;
    status?: string;
    limit?: number;
  } = {},
) {
  const search = new URLSearchParams();
  if (params.query?.trim()) {
    search.set('query', params.query.trim());
  }
  if (params.status?.trim()) {
    search.set('status', params.status.trim());
  }
  search.set('limit', String(params.limit ?? 25));

  return apiRequest<CatalogProvider[]>(
    `/v1/catalog/providers?${search.toString()}`,
  );
}

export async function fetchCatalogGames(
  params: {
    provider_code?: string;
    query?: string;
    status?: string;
    limit?: number;
  } = {},
) {
  const search = new URLSearchParams();
  if (params.provider_code?.trim()) {
    search.set('provider_code', params.provider_code.trim());
  }
  if (params.query?.trim()) {
    search.set('query', params.query.trim());
  }
  if (params.status?.trim()) {
    search.set('status', params.status.trim());
  }
  search.set('limit', String(params.limit ?? 100));

  return apiRequest<CatalogGame[]>(`/v1/catalog/games?${search.toString()}`);
}
