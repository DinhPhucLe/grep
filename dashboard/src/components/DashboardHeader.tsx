import { Badge, Group, Paper, Stack, Text, Title } from '@mantine/core';
import type { DashboardSnapshot } from '../contracts/dashboard';
import type { StreamStatus } from '../data/dashboardStream';

interface DashboardHeaderProps {
  session: DashboardSnapshot['session'];
  generatedAt: string;
  status: StreamStatus;
}

const formatTime = (value: string) =>
  new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  }).format(new Date(value));

const statusLabel = (status: StreamStatus): string => {
  switch (status) {
    case 'live':
      return 'Live';
    case 'connecting':
      return 'Connecting';
    case 'disconnected':
      return 'Disconnected';
    case 'mock':
      return 'Mock data';
  }
};

export function DashboardHeader({ session, generatedAt, status }: DashboardHeaderProps) {
  const sessionBadgeClass =
    session.state === 'active' ? 'status-badge status-badge--active' : 'status-badge status-badge--idle';

  const feedBadgeClass =
    status === 'live'
      ? 'status-badge status-badge--active'
      : status === 'mock'
        ? 'status-badge status-badge--mock'
        : 'status-badge status-badge--idle';

  return (
    <Paper className="dashboard-header" p={{ base: 'lg', sm: 'xl' }} radius={0} withBorder>
      <Group justify="space-between" align="flex-start" gap="lg">
        <Stack gap="xs">
          <Group gap="sm">
            <div className="brand-mark" aria-hidden="true">
              VO
            </div>
            <div>
              <Text className="eyebrow">Vibe Coding Observatory</Text>
              <Title order={1} className="section-title">
                Session evidence
              </Title>
            </div>
          </Group>
          <Group gap="xs" mt="xs">
            <Badge className={feedBadgeClass} variant="filled">
              {statusLabel(status)}
            </Badge>
            <Badge className={sessionBadgeClass} variant="filled">
              {session.state}
            </Badge>
            <Text className="secondary-text" fz="xs">
              {session.id}
            </Text>
          </Group>
        </Stack>

        <Stack gap="xs" align="flex-end">
          <Text className="secondary-text" fz="xs">
            Observed through {formatTime(session.observedThrough)}
          </Text>
          <Text className="secondary-text" fz="xs">
            Snapshot {formatTime(generatedAt)}
          </Text>
        </Stack>
      </Group>
    </Paper>
  );
}
