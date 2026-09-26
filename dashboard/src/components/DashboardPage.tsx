import { Container, Grid, SimpleGrid, Stack, Text, Title } from '@mantine/core';
import { dashboardLayout } from '../config/layout';
import type { DashboardSnapshot } from '../contracts/dashboard';
import { CodeHeatmap } from './heatmap/CodeHeatmap';
import { DashboardHeader } from './DashboardHeader';
import { MetricRenderer } from './metrics/MetricRenderer';
import { SessionSummary } from './SessionSummary';

export function DashboardPage({ snapshot }: { snapshot: DashboardSnapshot }) {
  const significant = snapshot.metrics.filter((metric) => metric.category === 'significant');
  const descriptive = snapshot.metrics.filter((metric) => metric.category === 'descriptive');

  return (
    <Container
      component="main"
      size={dashboardLayout.maxWidth}
      px={dashboardLayout.pagePadding}
      py={{ base: 'md', sm: 'xl' }}
    >
      <Stack gap={dashboardLayout.sectionGap}>
        <DashboardHeader session={snapshot.session} generatedAt={snapshot.generatedAt} />

        <section aria-labelledby="signals-heading">
          <Text className="eyebrow">Significant metrics</Text>
          <Title id="signals-heading" order={2} fz="xl" mt={4} mb="md">
            Observable session signals
          </Title>
          <SimpleGrid cols={dashboardLayout.significantColumns}>
            {significant.map((metric) => (
              <MetricRenderer key={metric.id} metric={metric} />
            ))}
          </SimpleGrid>
        </section>

        <Grid gap="xl" align="stretch">
          <Grid.Col span={{ base: 12, lg: 5 }}>
            <SessionSummary summary={snapshot.summary} />
          </Grid.Col>
          <Grid.Col span={{ base: 12, lg: 7 }}>
            <section aria-labelledby="descriptive-heading">
              <Text className="eyebrow">Session context</Text>
              <Title id="descriptive-heading" order={2} fz="xl" mt={4} mb="md">
                Descriptive numbers
              </Title>
              <SimpleGrid cols={dashboardLayout.descriptiveColumns}>
                {descriptive.map((metric) => (
                  <MetricRenderer key={metric.id} metric={metric} />
                ))}
              </SimpleGrid>
            </section>
          </Grid.Col>
        </Grid>

        <CodeHeatmap heatmap={snapshot.heatmap} />

        <Text ta="center" c="dimmed" fz="xs" pb="md">
          Descriptive metrics and code-change evidence do not update a cortisol signal.
        </Text>
      </Stack>
    </Container>
  );
}
