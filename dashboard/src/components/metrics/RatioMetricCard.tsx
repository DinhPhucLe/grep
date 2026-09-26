import { Group, RingProgress, Stack, Text } from '@mantine/core';
import { chartConfig } from '../../config/charts';
import type { RatioMetric } from '../../contracts/dashboard';
import { MetricCardShell } from '../shared/MetricCardShell';

export function RatioMetricCard({ metric }: { metric: RatioMetric }) {
  const { visualization } = metric;
  const range = visualization.maximum - visualization.minimum;
  const progress = range <= 0
    ? 0
    : ((visualization.value - visualization.minimum) / range) * 100;

  return (
    <MetricCardShell metric={metric}>
      <Group justify="space-between" align="center" mt="auto" wrap="nowrap">
        <RingProgress
          size={chartConfig.ratioSize}
          thickness={chartConfig.ratioThickness}
          roundCaps={false}
          sections={[{ value: progress, color: chartConfig.ratioColor }]}
          label={
            <Stack gap={0} align="center">
              <Text fw={700} fz="xl">
                {metric.display.primary}
              </Text>
              <Text className="secondary-text" fz="10px" tt="uppercase" fw={700}>
                accepted
              </Text>
            </Stack>
          }
          aria-label={`${metric.label}: ${metric.display.primary}`}
        />
        <Stack gap="xs" className="ratio-legend">
          {visualization.segments.map((segment, index) => (
            <Group key={segment.label} gap="xs" wrap="nowrap">
              <span
                className="ratio-legend__swatch"
                style={{
                  backgroundColor:
                    chartConfig.segmentColors[index] ?? chartConfig.segmentColors[2],
                }}
                aria-hidden="true"
              />
              <Text fz="xs" className="ratio-legend__label secondary-text">
                {segment.label}
              </Text>
              <Text fz="sm" fw={700} ml="auto">
                {segment.value}
              </Text>
            </Group>
          ))}
          {metric.display.secondary ? (
            <Text fz="xs" className="secondary-text" mt={2}>
              {metric.display.secondary}
            </Text>
          ) : null}
        </Stack>
      </Group>
    </MetricCardShell>
  );
}
