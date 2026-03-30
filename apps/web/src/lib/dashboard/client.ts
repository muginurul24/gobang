import { apiRequest } from '$lib/auth/client';

export type DashboardStoreMetrics = {
  accessible_store_count: number;
  balance_total: string;
  pending_qris_count: number;
  success_today_count: number;
  expired_today_count: number;
  monthly_store_income: string;
};

export type DashboardPlatformMetrics = {
  platform_income_today: string;
  platform_income_month: string;
  total_store_count: number;
  pending_withdraw_count: number;
  upstream_error_rate_24h: number;
  callback_failure_rate_24h: number;
};

export type DashboardSummary = {
  role: string;
  store_metrics?: DashboardStoreMetrics | null;
  platform_metrics?: DashboardPlatformMetrics | null;
};

export async function fetchDashboardCards() {
  return apiRequest<DashboardSummary>('/v1/dashboard/cards');
}
