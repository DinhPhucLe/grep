const MONTHS = [
  'jan',
  'feb',
  'mar',
  'apr',
  'may',
  'jun',
  'jul',
  'aug',
  'sep',
  'oct',
  'nov',
  'dec',
] as const;

/** Formats ISO date or YYYY-MM-DD as "1 jan 2026". */
export function formatReadableDate(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed) return raw;
  const datePart = trimmed.includes('T') ? trimmed.slice(0, 10) : trimmed.slice(0, 10);
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(datePart);
  if (!match) return raw;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  if (month < 1 || month > 12 || day < 1 || day > 31) return raw;
  return `${day} ${MONTHS[month - 1]} ${year}`;
}

/** Truncates long IDs to first 6 characters + ellipsis. */
export function truncateId(id: string): string {
  if (id.length <= 6) return id;
  return `${id.slice(0, 6)}…`;
}
