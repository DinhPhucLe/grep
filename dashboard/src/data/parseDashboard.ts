import {
  metricStatuses,
  type DashboardMetric,
  type DashboardSnapshot,
  type HeatmapFile,
  type HeatmapIntensity,
  type HeatmapRow,
  type HeatmapRowState,
  type HistogramBin,
  type MetricBase,
  type RatioSegment,
} from '../contracts/dashboard';

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

const expectRecord = (value: unknown, path: string): Record<string, unknown> => {
  if (!isRecord(value)) throw new Error(`${path} must be an object`);
  return value;
};

const expectString = (value: unknown, path: string): string => {
  if (typeof value !== 'string') throw new Error(`${path} must be a string`);
  return value;
};

const expectNumber = (value: unknown, path: string): number => {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    throw new Error(`${path} must be a finite number`);
  }
  return value;
};

const expectArray = (value: unknown, path: string): unknown[] => {
  if (!Array.isArray(value)) throw new Error(`${path} must be an array`);
  return value;
};

const expectNullableNumber = (value: unknown, path: string): number | null => {
  if (value === null) return null;
  return expectNumber(value, path);
};

const expectOneOf = <T extends string>(
  value: unknown,
  options: readonly T[],
  path: string,
): T => {
  if (typeof value !== 'string' || !options.includes(value as T)) {
    throw new Error(`${path} must be one of: ${options.join(', ')}`);
  }
  return value as T;
};

function parseMetricBase(raw: Record<string, unknown>, path: string): MetricBase {
  const display = expectRecord(raw.display, `${path}.display`);
  const secondary = display.secondary;
  if (secondary !== undefined && typeof secondary !== 'string') {
    throw new Error(`${path}.display.secondary must be a string when provided`);
  }

  return {
    id: expectString(raw.id, `${path}.id`),
    category: expectOneOf(raw.category, ['significant', 'descriptive'], `${path}.category`),
    label: expectString(raw.label, `${path}.label`),
    ...(raw.description === undefined
      ? {}
      : { description: expectString(raw.description, `${path}.description`) }),
    status: expectOneOf(raw.status, metricStatuses, `${path}.status`),
    display: {
      primary: expectString(display.primary, `${path}.display.primary`),
      ...(secondary === undefined ? {} : { secondary }),
    },
  };
}

function parseRatioSegment(raw: unknown, path: string): RatioSegment {
  const value = expectRecord(raw, path);
  return {
    label: expectString(value.label, `${path}.label`),
    value: expectNumber(value.value, `${path}.value`),
  };
}

function parseHistogramBin(raw: unknown, path: string): HistogramBin {
  const value = expectRecord(raw, path);
  return {
    label: expectString(value.label, `${path}.label`),
    value: expectNumber(value.value, `${path}.value`),
  };
}

function parseMetric(raw: unknown, path: string): DashboardMetric {
  const metric = expectRecord(raw, path);
  const base = parseMetricBase(metric, path);
  const visualization = expectRecord(metric.visualization, `${path}.visualization`);
  const type = expectOneOf(
    visualization.type,
    ['stat', 'ratio', 'histogram'],
    `${path}.visualization.type`,
  );

  if (type === 'stat') {
    return { ...base, visualization: { type } };
  }

  if (type === 'ratio') {
    const segments = expectArray(
      visualization.segments,
      `${path}.visualization.segments`,
    ).map((segment, index) =>
      parseRatioSegment(segment, `${path}.visualization.segments[${index}]`),
    );
    return {
      ...base,
      visualization: {
        type,
        value: expectNumber(visualization.value, `${path}.visualization.value`),
        minimum: expectNumber(visualization.minimum, `${path}.visualization.minimum`),
        maximum: expectNumber(visualization.maximum, `${path}.visualization.maximum`),
        segments,
      },
    };
  }

  const median = expectRecord(visualization.median, `${path}.visualization.median`);
  if (median.unit !== 'milliseconds') {
    throw new Error(`${path}.visualization.median.unit must be milliseconds`);
  }
  const bins = expectArray(visualization.bins, `${path}.visualization.bins`).map(
    (bin, index) => parseHistogramBin(bin, `${path}.visualization.bins[${index}]`),
  );
  return {
    ...base,
    visualization: {
      type,
      median: {
        value: expectNumber(median.value, `${path}.visualization.median.value`),
        unit: 'milliseconds',
      },
      bins,
    },
  };
}

function parseHeatmapRow(raw: unknown, path: string): HeatmapRow {
  const row = expectRecord(raw, path);
  const state = expectOneOf(
    row.state,
    ['context', 'retained_change', 'deleted_or_replaced'],
    `${path}.state`,
  ) as HeatmapRowState;
  const intensity = expectNumber(row.intensity, `${path}.intensity`);
  if (![0, 1, 2, 3, 4].includes(intensity)) {
    throw new Error(`${path}.intensity must be an integer from 0 through 4`);
  }

  return {
    rowId: expectString(row.rowId, `${path}.rowId`),
    oldLineNumber: expectNullableNumber(row.oldLineNumber, `${path}.oldLineNumber`),
    newLineNumber: expectNullableNumber(row.newLineNumber, `${path}.newLineNumber`),
    content: expectString(row.content, `${path}.content`),
    state,
    churnCount: expectNumber(row.churnCount, `${path}.churnCount`),
    intensity: intensity as HeatmapIntensity,
  };
}

function parseHeatmapFile(raw: unknown, path: string): HeatmapFile {
  const file = expectRecord(raw, path);
  return {
    path: expectString(file.path, `${path}.path`),
    rows: expectArray(file.rows, `${path}.rows`).map((row, index) =>
      parseHeatmapRow(row, `${path}.rows[${index}]`),
    ),
  };
}

export function parseDashboardSnapshot(raw: unknown): DashboardSnapshot {
  const snapshot = expectRecord(raw, 'dashboard');
  if (snapshot.schemaVersion !== 'dashboard.v1') {
    throw new Error('dashboard.schemaVersion must be dashboard.v1');
  }

  const session = expectRecord(snapshot.session, 'dashboard.session');
  const summary = expectRecord(snapshot.summary, 'dashboard.summary');
  const heatmap = expectRecord(snapshot.heatmap, 'dashboard.heatmap');

  return {
    schemaVersion: 'dashboard.v1',
    sequence: expectNumber(snapshot.sequence, 'dashboard.sequence'),
    generatedAt: expectString(snapshot.generatedAt, 'dashboard.generatedAt'),
    session: {
      id: expectString(session.id, 'dashboard.session.id'),
      startedAt: expectString(session.startedAt, 'dashboard.session.startedAt'),
      observedThrough: expectString(
        session.observedThrough,
        'dashboard.session.observedThrough',
      ),
      state: expectOneOf(session.state, ['active', 'complete'], 'dashboard.session.state'),
    },
    metrics: expectArray(snapshot.metrics, 'dashboard.metrics').map((metric, index) =>
      parseMetric(metric, `dashboard.metrics[${index}]`),
    ),
    summary: {
      status: expectOneOf(summary.status, ['available', 'unknown'], 'dashboard.summary.status'),
      bullets: expectArray(summary.bullets, 'dashboard.summary.bullets').map((bullet, index) =>
        expectString(bullet, `dashboard.summary.bullets[${index}]`),
      ),
    },
    heatmap: {
      mode: expectOneOf(heatmap.mode, ['history', 'final_diff'], 'dashboard.heatmap.mode'),
      files: expectArray(heatmap.files, 'dashboard.heatmap.files').map((file, index) =>
        parseHeatmapFile(file, `dashboard.heatmap.files[${index}]`),
      ),
    },
  };
}
