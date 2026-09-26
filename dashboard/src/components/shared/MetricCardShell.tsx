import { Badge, Group, Paper, Stack, Text } from '@mantine/core';
import type { MetricBase } from '../../contracts/dashboard';

interface MetricCardShellProps {
  metric: MetricBase;
  children?: React.ReactNode;
  className?: string;
}

export function MetricCardShell({
  metric,
  children,
  className = 'metric-card',
}: MetricCardShellProps) {
  const statusLabel = metric.status.replace('_', ' ');

  return (
    <Paper className={className} p="lg" radius={0} withBorder>
      <Stack gap="md" h="100%">
        <Group justify="space-between" align="flex-start" wrap="nowrap">
          <div>
            <Text fw={700} tt="uppercase" style={{ letterSpacing: '1px' }}>
              {metric.label}
            </Text>
            {metric.description ? (
              <Text className="secondary-text" fz="xs" mt={3} lh={1.45}>
                {metric.description}
              </Text>
            ) : null}
          </div>
          {metric.status !== 'available' ? (
            <Badge className="status-badge status-badge--unavailable" variant="filled" size="xs">
              {statusLabel}
            </Badge>
          ) : null}
        </Group>
        {children}
      </Stack>
    </Paper>
  );
}
