import { Container, Group, SimpleGrid, Stack, Text, Title } from '@mantine/core';
import { dashboardLayout } from '../../config/layout';
import type { EmployeePracticeView } from '../../contracts/practice';
import { MetricRenderer } from '../metrics/MetricRenderer';
import { ActivityCalendarHeatmap } from './ActivityCalendarHeatmap';
import { OutcomePieCard } from './OutcomePieCard';

export function EmployeePracticePage({ view }: { view: EmployeePracticeView }) {
  return (
    <Container
      component="main"
      size={dashboardLayout.maxWidth}
      px={dashboardLayout.pagePadding}
      py={{ base: 'md', sm: 'xl' }}
    >
      <Stack gap={dashboardLayout.sectionGap}>
        <div>
          <Text className="eyebrow">Employee practice</Text>
          <Title order={1} className="section-title">
            {view.subject.practice}
          </Title>
          <Text className="secondary-text">
            User {view.subject.userId} · Year {view.subject.year}
          </Text>
        </div>

        <ActivityCalendarHeatmap calendar={view.activityCalendar} year={view.subject.year} />

        <Group align="stretch" grow preventGrowOverflow={false}>
          <OutcomePieCard pie={view.outcomePie} />
          <SimpleGrid cols={{ base: 1, sm: 2 }} style={{ flex: 1 }}>
            {view.metrics.map((metric) => (
              <MetricRenderer key={metric.id} metric={metric} />
            ))}
          </SimpleGrid>
        </Group>

        {view.summary ? (
          <Stack gap="xs">
            {view.summary.bullets.map((bullet) => (
              <Text key={bullet}>{bullet}</Text>
            ))}
          </Stack>
        ) : null}
      </Stack>
    </Container>
  );
}
