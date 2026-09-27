import { useState } from 'react';
import { Button, Container, Group, SimpleGrid, Stack, Text, Title } from '@mantine/core';
import { dashboardLayout } from '../../config/layout';
import type { OrgKnowledgeView } from '../../contracts/knowledge';
import type { OrgPracticeView } from '../../contracts/practice';
import { formatReadableDate, truncateId } from '../../utils/formatPractice';
import { LearningDotsCard } from '../knowledge/LearningDotsCard';
import { TopicNetworkCard } from '../knowledge/TopicNetworkCard';
import { MetricRenderer } from '../metrics/MetricRenderer';
import { CodebaseTreemapCard } from './CodebaseTreemapCard';
import { PracticeTimeseriesCard } from './PracticeTimeseriesCard';

type Mode = 'practice' | 'knowledge';

function orgSubtitle(view: OrgPracticeView): string {
  const { organizationName, organizationId, from, to } = view.subject;
  const idPart = truncateId(organizationId);
  const range = `${formatReadableDate(from)} → ${formatReadableDate(to)}`;
  if (organizationName) {
    return `${organizationName} · ${idPart} · ${range}`;
  }
  return `Org ${idPart} · ${range}`;
}

function knowledgeSubtitle(view: OrgKnowledgeView): string {
  const { organizationName, organizationId, from, to } = view.subject;
  const idPart = truncateId(organizationId);
  const range = `${formatReadableDate(from)} → ${formatReadableDate(to)}`;
  if (organizationName) {
    return `${organizationName} · ${idPart} · ${range}`;
  }
  return `Org ${idPart} · ${range}`;
}

export function OrgPracticePage({
  view,
  knowledge,
}: {
  view: OrgPracticeView;
  knowledge?: OrgKnowledgeView | null;
}) {
  const [mode, setMode] = useState<Mode>(knowledge ? 'knowledge' : 'practice');
  const showKnowledge = mode === 'knowledge' && knowledge != null;

  return (
    <Container
      component="main"
      size={dashboardLayout.maxWidth}
      px={dashboardLayout.pagePadding}
      py={{ base: 'md', sm: 'xl' }}
    >
      <Stack gap={dashboardLayout.sectionGap}>
        <div>
          <Text className="eyebrow">
            {showKnowledge ? 'Organization knowledge' : 'Organization practice'}
          </Text>
          <Title order={1} className="section-title">
            {showKnowledge ? 'Learning edges' : view.subject.practice}
          </Title>
          <Text className="secondary-text">
            {showKnowledge ? knowledgeSubtitle(knowledge) : orgSubtitle(view)}
          </Text>
        </div>

        <Group gap="xs" role="tablist" aria-label="Organization view mode">
          <Button
            role="tab"
            aria-selected={mode === 'practice'}
            data-selected={mode === 'practice'}
            variant="default"
            size="compact-sm"
            className="file-selector-btn"
            onClick={() => setMode('practice')}
          >
            Practice
          </Button>
          <Button
            role="tab"
            aria-selected={mode === 'knowledge'}
            data-selected={mode === 'knowledge'}
            variant="default"
            size="compact-sm"
            className="file-selector-btn"
            disabled={knowledge == null}
            onClick={() => setMode('knowledge')}
          >
            Knowledge
          </Button>
        </Group>

        {showKnowledge ? (
          <>
            <SimpleGrid cols={dashboardLayout.significantColumns}>
              {knowledge.metrics.map((metric) => (
                <MetricRenderer key={metric.id} metric={metric} />
              ))}
            </SimpleGrid>
            <TopicNetworkCard network={knowledge.topicNetwork} />
            <LearningDotsCard dots={knowledge.learningDots} />
            {knowledge.summary ? (
              <Stack gap="xs">
                {knowledge.summary.bullets.map((bullet) => (
                  <Text key={bullet}>{bullet}</Text>
                ))}
              </Stack>
            ) : null}
          </>
        ) : (
          <>
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
          </>
        )}
      </Stack>
    </Container>
  );
}
