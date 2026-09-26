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
            <Text className="secondary-text" fz="xs">
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
          gridColor={chartConfig.gridStroke}
          textColor="#000000"
          yAxisProps={{ allowDecimals: false, width: 24 }}
          barProps={{
            radius: 0,
            isAnimationActive: false,
            stroke: chartConfig.barStroke,
            strokeWidth: chartConfig.barStrokeWidth,
          }}
          aria-label={`${metric.label} distribution`}
        />
      </Stack>
    </MetricCardShell>
  );
}
