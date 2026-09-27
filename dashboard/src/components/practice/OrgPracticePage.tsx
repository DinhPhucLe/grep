import { Container, SimpleGrid, Stack, Text, Title } from '@mantine/core';
import { dashboardLayout } from '../../config/layout';
import type { OrgPracticeView } from '../../contracts/practice';
import { MetricRenderer } from '../metrics/MetricRenderer';
import { CodebaseTreemapCard } from './CodebaseTreemapCard';
import { PracticeTimeseriesCard } from './PracticeTimeseriesCard';

export function OrgPracticePage({ view }: { view: OrgPracticeView }) {
  return (
    <Container
      component="main"
      size={dashboardLayout.maxWidth}
      px={dashboardLayout.pagePadding}
      py={{ base: 'md', sm: 'xl' }}
    >
      <Stack gap={dashboardLayout.sectionGap}>
        <div>
          <Text className="eyebrow">Organization practice</Text>
          <Title order={1} className="section-title">
            {view.subject.practice}
          </Title>
          <Text className="secondary-text">
            Org {view.subject.organizationId} · {view.subject.from} → {view.subject.to}
          </Text>
        </div>

        <PracticeTimeseriesCard timeseries={view.timeseries} />

        <SimpleGrid cols={{ base: 1, md: 2 }}>
          {view.codebaseTreemaps.map((treemap) => (
            <CodebaseTreemapCard key={treemap.projectId} treemap={treemap} />
          ))}
        </SimpleGrid>

        <SimpleGrid cols={dashboardLayout.significantColumns}>
          {view.metrics.map((metric) => (
            <MetricRenderer key={metric.id} metric={metric} />
          ))}
        </SimpleGrid>

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
