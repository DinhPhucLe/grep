import {
  ActionIcon,
  Badge,
  Group,
  Paper,
  Stack,
  Text,
  Title,
  Tooltip,
  useComputedColorScheme,
  useMantineColorScheme,
} from '@mantine/core';
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
  const { setColorScheme } = useMantineColorScheme();
  const colorScheme = useComputedColorScheme('dark');
  const nextScheme = colorScheme === 'dark' ? 'light' : 'dark';

  return (
    <Paper className="dashboard-header" p={{ base: 'lg', sm: 'xl' }} radius="xl" withBorder>
      <Group justify="space-between" align="flex-start" gap="lg">
        <Stack gap="xs">
          <Group gap="sm">
            <div className="brand-mark" aria-hidden="true">
              VO
            </div>
            <div>
              <Text className="eyebrow">Vibe Coding Observatory</Text>
              <Title order={1}>Session evidence</Title>
            </div>
          </Group>
          <Group gap="xs" mt="xs">
            <Badge color="orange" variant="light">
              Mock data
            </Badge>
            <Badge color={session.state === 'active' ? 'teal' : 'gray'} variant="dot">
              {session.state}
            </Badge>
            <Text c="dimmed" fz="xs">
              {session.id}
            </Text>
          </Group>
        </Stack>

        <Stack gap="xs" align="flex-end">
          <Tooltip label={`Switch to ${nextScheme} mode`}>
            <ActionIcon
              variant="subtle"
              color="gray"
              size="lg"
              aria-label={`Switch to ${nextScheme} mode`}
              onClick={() => setColorScheme(nextScheme)}
            >
              <span aria-hidden="true">{colorScheme === 'dark' ? '☀' : '☾'}</span>
            </ActionIcon>
          </Tooltip>
          <Text c="dimmed" fz="xs">
            Observed through {formatTime(session.observedThrough)}
          </Text>
          <Text c="dimmed" fz="xs">
            Snapshot {formatTime(generatedAt)}
          </Text>
        </Stack>
      </Group>
    </Paper>
  );
}
