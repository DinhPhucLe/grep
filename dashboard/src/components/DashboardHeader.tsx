import { Badge, Group, Paper, Stack, Text, Title } from '@mantine/core';
import type { DashboardSnapshot } from '../contracts/dashboard';

interface DashboardHeaderProps {
  session: DashboardSnapshot['session'];
  generatedAt: string;
}

const formatTime = (value: string) =>
  new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  }).format(new Date(value));

export function DashboardHeader({ session, generatedAt }: DashboardHeaderProps) {
  const sessionBadgeClass =
    session.state === 'active' ? 'status-badge status-badge--active' : 'status-badge status-badge--idle';

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
            <Badge className="status-badge status-badge--mock" variant="filled">
              Mock data
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
