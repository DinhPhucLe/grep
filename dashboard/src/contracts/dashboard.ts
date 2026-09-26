export const metricStatuses = ['available', 'no_data', 'unknown', 'invalid'] as const;
export type MetricStatus = (typeof metricStatuses)[number];

export interface MetricDisplay {
  primary: string;
  secondary?: string;
}

export interface MetricBase {
  id: string;
  category: 'significant' | 'descriptive';
  label: string;
  description?: string;
  status: MetricStatus;
  display: MetricDisplay;
}

export interface StatMetric extends MetricBase {
  visualization: {
    type: 'stat';
  };
}

export interface RatioSegment {
  label: string;
  value: number;
}

export interface RatioMetric extends MetricBase {
  visualization: {
    type: 'ratio';
    value: number;
    minimum: number;
    maximum: number;
    segments: RatioSegment[];
  };
}

export interface HistogramBin {
  label: string;
  value: number;
}

export interface HistogramMetric extends MetricBase {
  visualization: {
    type: 'histogram';
    median: {
      value: number;
      unit: 'milliseconds';
    };
    bins: HistogramBin[];
  };
}

export type DashboardMetric = StatMetric | RatioMetric | HistogramMetric;

export type HeatmapRowState = 'context' | 'retained_change' | 'deleted_or_replaced';
export type HeatmapIntensity = 0 | 1 | 2 | 3 | 4;

export interface HeatmapRow {
  rowId: string;
  oldLineNumber: number | null;
  newLineNumber: number | null;
  content: string;
  state: HeatmapRowState;
  churnCount: number;
  intensity: HeatmapIntensity;
}

export interface HeatmapFile {
  path: string;
  rows: HeatmapRow[];
}

export interface CodeHeatmap {
  mode: 'history' | 'final_diff';
  files: HeatmapFile[];
}

export interface DashboardSnapshot {
  schemaVersion: 'dashboard.v1';
  sequence: number;
  generatedAt: string;
  session: {
    id: string;
    startedAt: string;
    observedThrough: string;
    state: 'active' | 'complete';
  };
  metrics: DashboardMetric[];
  summary: {
    status: 'available' | 'unknown';
    bullets: string[];
  };
  heatmap: CodeHeatmap;
}
