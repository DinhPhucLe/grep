import type {
  DashboardMetric,
  HistogramMetric,
  RatioMetric,
  StatMetric,
} from '../../contracts/dashboard';
import { HistogramMetricCard } from './HistogramMetricCard';
import { RatioMetricCard } from './RatioMetricCard';
import { StatMetricCard } from './StatMetricCard';

export function MetricRenderer({ metric }: { metric: DashboardMetric }) {
  switch (metric.visualization.type) {
    case 'stat':
      return <StatMetricCard metric={metric as StatMetric} />;
    case 'ratio':
      return <RatioMetricCard metric={metric as RatioMetric} />;
    case 'histogram':
      return <HistogramMetricCard metric={metric as HistogramMetric} />;
  }
}
