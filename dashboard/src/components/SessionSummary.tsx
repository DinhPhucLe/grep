import { Badge, List, Paper, Stack, Text, Title } from '@mantine/core';
import type { DashboardSnapshot } from '../contracts/dashboard';

export function SessionSummary({
  summary,
}: {
  summary: DashboardSnapshot['summary'];
}) {
  return (
    <Paper className="summary-panel" p="xl" radius="xl" withBorder h="100%">
      <Stack gap="lg">
        <div>
          <Text className="eyebrow">Qualitative evidence</Text>
          <Title order={2} fz="xl" mt={4}>
            Session summary
          </Title>
        </div>
        {summary.status === 'available' ? (
          <List spacing="md" className="summary-list">
            {summary.bullets.map((bullet) => (
              <List.Item key={bullet}>
                <Text fz="sm" lh={1.55}>
                  {bullet}
                </Text>
              </List.Item>
            ))}
          </List>
        ) : (
          <Badge color="gray" variant="light">
            Summary unavailable
          </Badge>
        )}
      </Stack>
    </Paper>
  );
}
