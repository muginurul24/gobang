export function matchesDateTimeRange(
  value: string | null | undefined,
  start: string,
  end: string,
) {
  if (start.trim() === '' && end.trim() === '') {
    return true;
  }

  if (!value) {
    return false;
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return false;
  }

  const startedAt =
    start.trim() === '' ? Number.NEGATIVE_INFINITY : new Date(start).getTime();
  const endedAt =
    end.trim() === '' ? Number.POSITIVE_INFINITY : new Date(end).getTime();

  return date.getTime() >= startedAt && date.getTime() <= endedAt;
}

export function paginateItems<T>(items: T[], page: number, pageSize: number) {
  const safePageSize =
    Number.isFinite(pageSize) && pageSize > 0 ? Math.trunc(pageSize) : 12;
  const totalPages = Math.max(1, Math.ceil(items.length / safePageSize));
  const currentPage = Math.min(Math.max(1, Math.trunc(page)), totalPages);
  const startIndex = (currentPage - 1) * safePageSize;
  const endIndex = Math.min(items.length, startIndex + safePageSize);

  return {
    currentPage,
    totalPages,
    startIndex,
    endIndex,
    items: items.slice(startIndex, endIndex),
  };
}
