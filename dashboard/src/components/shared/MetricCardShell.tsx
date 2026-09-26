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
    <Paper className={className} p="lg" radius="lg" withBorder>
      <Stack gap="md" h="100%">
        <Group justify="space-between" align="flex-start" wrap="nowrap">
          <div>
            <Text fw={650}>{metric.label}</Text>
            {metric.description ? (
              <Text c="dimmed" fz="xs" mt={3} lh={1.45}>
                {metric.description}
              </Text>
            ) : null}
          </div>
          {metric.status !== 'available' ? (
            <Badge color="gray" variant="light" size="xs">
              {statusLabel}
            </Badge>
          ) : null}
        </Group>
        {children}
      </Stack>
    </Paper>
  );
}
