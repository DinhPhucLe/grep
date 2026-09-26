import { BarChart } from '@mantine/charts';
import { Group, Stack, Text } from '@mantine/core';
import { chartConfig } from '../../config/charts';
import type { HistogramMetric } from '../../contracts/dashboard';
import { MetricCardShell } from '../shared/MetricCardShell';

export function HistogramMetricCard({ metric }: { metric: HistogramMetric }) {
  return (
    <MetricCardShell metric={metric}>
      <Stack gap="sm" mt="auto">
        <Group gap="xs" align="baseline">
          <Text className="signal-value">{metric.display.primary}</Text>
          {metric.display.secondary ? (
            <Text c="dimmed" fz="xs">
              {metric.display.secondary}
            </Text>
          ) : null}
        </Group>
        <BarChart
          h={chartConfig.histogramHeight}
          data={metric.visualization.bins}
          dataKey="label"
          series={[{ name: 'value', label: 'Observations', color: chartConfig.histogramColor }]}
          withLegend={false}
          withTooltip
          gridAxis="y"
          tickLine="none"
          yAxisProps={{ allowDecimals: false, width: 24 }}
          barProps={{ radius: 4, isAnimationActive: false }}
          aria-label={`${metric.label} distribution`}
        />
      </Stack>
    </MetricCardShell>
  );
}
