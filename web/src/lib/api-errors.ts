function objectRecord(value: unknown): Record<string, unknown> | undefined {
  return typeof value === 'object' && value !== null
    ? value as Record<string, unknown>
    : undefined;
}

export function apiErrorMessage(
  error: unknown,
  fallback?: string,
  fields: readonly ('error' | 'message')[] = ['error'],
): string {
  const record = objectRecord(error);
  const response = objectRecord(record?.response);
  const data = objectRecord(response?.data);
  for (const field of fields) {
    const value = data?.[field];
    if (typeof value === 'string' && value) return value;
  }
  if (fallback !== undefined) return fallback;
  if (typeof record?.message === 'string' && record.message) return record.message;
  return typeof error === 'string' ? error : '';
}
