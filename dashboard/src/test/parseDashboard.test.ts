import { describe, expect, it } from 'vitest';
import mockDashboard from '../mocks/dashboard.mock.json';
import { parseDashboardSnapshot } from '../data/parseDashboard';

describe('parseDashboardSnapshot', () => {
  it('parses the complete mock snapshot', () => {
    const snapshot = parseDashboardSnapshot(mockDashboard);

    expect(snapshot.schemaVersion).toBe('dashboard.v1');
    expect(snapshot.metrics).toHaveLength(14);
    expect(snapshot.heatmap.files).toHaveLength(3);
    expect(snapshot.summary.bullets).toHaveLength(5);
  });

  it('rejects unsupported schema versions', () => {
    expect(() =>
      parseDashboardSnapshot({ ...mockDashboard, schemaVersion: 'dashboard.v2' }),
    ).toThrow('dashboard.schemaVersion must be dashboard.v1');
  });

  it('rejects unknown visualization types', () => {
    const invalid = structuredClone(mockDashboard) as Record<string, unknown> & {
      metrics: Array<Record<string, unknown>>;
    };
    invalid.metrics[0].visualization = { type: 'gauge' };

    expect(() => parseDashboardSnapshot(invalid)).toThrow(
      'dashboard.metrics[0].visualization.type must be one of',
    );
  });

  it('rejects an out-of-range heatmap intensity', () => {
    const invalid = structuredClone(mockDashboard) as Record<string, unknown> & {
      heatmap: { files: Array<{ rows: Array<Record<string, unknown>> }> };
    };
    invalid.heatmap.files[0].rows[0].intensity = 7;

    expect(() => parseDashboardSnapshot(invalid)).toThrow(
      'dashboard.heatmap.files[0].rows[0].intensity must be an integer from 0 through 4',
    );
  });
});
