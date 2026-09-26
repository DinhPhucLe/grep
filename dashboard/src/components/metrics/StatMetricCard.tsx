import { Stack, Text } from '@mantine/core';
import type { StatMetric } from '../../contracts/dashboard';
import { MetricCardShell } from '../shared/MetricCardShell';

export function StatMetricCard({ metric }: { metric: StatMetric }) {
  return (
    <MetricCardShell metric={metric} className="stat-card">
      <Stack gap={2} mt="auto">
        <Text className="stat-card__value">{metric.display.primary}</Text>
        {metric.display.secondary ? (
          <Text c="dimmed" fz="xs">
            {metric.display.secondary}
          </Text>
        ) : null}
      </Stack>
    </MetricCardShell>
  );
}
