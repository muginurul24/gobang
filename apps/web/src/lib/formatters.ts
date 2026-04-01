import { parseMoney } from '$lib/stores/client';

export function formatCurrency(
  value: string | number | null | undefined,
  options: { compact?: boolean } = {},
) {
  return new Intl.NumberFormat('id-ID', {
    style: 'currency',
    currency: 'IDR',
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
    notation: options.compact ? 'compact' : 'standard',
  }).format(parseMoney(value));
}

export function formatNumber(value: number | null | undefined) {
  return new Intl.NumberFormat('id-ID', {
    maximumFractionDigits: 0,
  }).format(Number(value ?? 0));
}

export function formatPercent(
  value: number | null | undefined,
  options: { digits?: number } = {},
) {
  const digits = options.digits ?? 1;
  const normalized = Number.isFinite(Number(value)) ? Number(value) : 0;

  return `${normalized.toFixed(digits)}%`;
}

export function formatDateTime(value: string | null | undefined) {
  if (!value) {
    return '-';
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '-';
  }

  return new Intl.DateTimeFormat('id-ID', {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

export function formatRelativeShort(value: string | null | undefined) {
  if (!value) {
    return '-';
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '-';
  }

  const deltaSeconds = Math.round((date.getTime() - Date.now()) / 1000);
  const absSeconds = Math.abs(deltaSeconds);

  if (absSeconds < 60) {
    return deltaSeconds >= 0 ? 'sebentar lagi' : 'baru saja';
  }

  const formatter = new Intl.RelativeTimeFormat('id-ID', { numeric: 'auto' });

  if (absSeconds < 3600) {
    return formatter.format(Math.round(deltaSeconds / 60), 'minute');
  }

  if (absSeconds < 86400) {
    return formatter.format(Math.round(deltaSeconds / 3600), 'hour');
  }

  return formatter.format(Math.round(deltaSeconds / 86400), 'day');
}

export function safeList<T>(value: T[] | null | undefined) {
  return Array.isArray(value) ? value : [];
}

export function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value));
}
