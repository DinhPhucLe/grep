import { Badge, List, Paper, Stack, Text, Title } from '@mantine/core';
import type { DashboardSnapshot } from '../contracts/dashboard';

export function SessionSummary({
  summary,
}: {
  summary: DashboardSnapshot['summary'];
}) {
  return (
    <Paper className="summary-panel" p="xl" radius={0} withBorder h="100%">
      <Stack gap="lg">
        <div>
          <Text className="eyebrow">Qualitative evidence</Text>
          <Title order={2} fz="xl" mt={4} className="section-title">
            Session summary
          </Title>
        </div>
        {summary.status === 'available' ? (
          <List spacing="md" className="summary-list" listStyleType="square">
            {summary.bullets.map((bullet) => (
              <List.Item key={bullet}>
                <Text fz="sm" lh={1.55}>
                  {bullet}
                </Text>
              </List.Item>
            ))}
          </List>
        ) : (
          <Badge className="status-badge status-badge--unavailable" variant="filled">
            Summary unavailable
          </Badge>
        )}
      </Stack>
    </Paper>
  );
}
