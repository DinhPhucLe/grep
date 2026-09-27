import { describe, expect, it } from 'vitest';
import { formatReadableDate, truncateId } from '../utils/formatPractice';

describe('formatReadableDate', () => {
  it('formats RFC3339 timestamps', () => {
    expect(formatReadableDate('2026-01-01T00:00:00Z')).toBe('1 jan 2026');
  });

  it('formats YYYY-MM-DD', () => {
    expect(formatReadableDate('2026-09-27')).toBe('27 sep 2026');
  });
});

describe('truncateId', () => {
  it('truncates long ids to first 6 characters', () => {
    expect(truncateId('660100000000000004000000')).toBe('660100…');
  });

  it('leaves short ids alone', () => {
    expect(truncateId('abc')).toBe('abc');
  });
});
