import { useState } from 'react';
import { Button, Container, Group, SimpleGrid, Stack, Text, Title } from '@mantine/core';
import { dashboardLayout } from '../../config/layout';
import type { EmployeeKnowledgeView } from '../../contracts/knowledge';
import type { EmployeePracticeView } from '../../contracts/practice';
import { truncateId } from '../../utils/formatPractice';
import { MetricRenderer } from '../metrics/MetricRenderer';
import { ActivityCalendarHeatmap } from './ActivityCalendarHeatmap';
import { OutcomePieCard } from './OutcomePieCard';

type Mode = 'quizzes' | 'edges';

function employeeSubtitle(view: EmployeePracticeView): string {
  const { userName, userId, year } = view.subject;
  if (userName) {
    return `${userName} · Year ${year}`;
  }
  return `User ${truncateId(userId)} · Year ${year}`;
}

export function EmployeePracticePage({
  view,
  knowledge,
}: {
  view: EmployeePracticeView;
  knowledge?: EmployeeKnowledgeView | null;
}) {
  const [mode, setMode] = useState<Mode>('quizzes');
  const showEdges = mode === 'edges' && knowledge != null;
  const calendar = showEdges ? knowledge.activityCalendar : view.activityCalendar;
  const year = showEdges ? knowledge.subject.year : view.subject.year;
  const metrics = showEdges ? knowledge.metrics : view.metrics;
  const summary = showEdges ? knowledge.summary : view.summary;

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
          <Text className="secondary-text">{employeeSubtitle(view)}</Text>
        </div>

        <Group gap="xs" role="tablist" aria-label="Calendar mode">
          <Button
            role="tab"
            aria-selected={mode === 'quizzes'}
            data-selected={mode === 'quizzes'}
            variant="default"
            size="compact-sm"
            className="file-selector-btn"
            onClick={() => setMode('quizzes')}
          >
            Quizzes
          </Button>
          <Button
            role="tab"
            aria-selected={mode === 'edges'}
            data-selected={mode === 'edges'}
            variant="default"
            size="compact-sm"
            className="file-selector-btn"
            disabled={knowledge == null}
            onClick={() => setMode('edges')}
          >
            Edges
          </Button>
        </Group>

        <ActivityCalendarHeatmap
          calendar={calendar}
          year={year}
          title={showEdges ? `Learning edges ${year}` : `Practice calendar ${year}`}
          ariaLabel={
            showEdges
              ? `Learning edge calendar for ${year}`
              : `Practice activity calendar for ${year}`
          }
        />

        {showEdges ? (
          <SimpleGrid cols={{ base: 1, sm: 2 }}>
            {metrics.map((metric) => (
              <MetricRenderer key={metric.id} metric={metric} />
            ))}
          </SimpleGrid>
        ) : (
          <Group align="stretch" grow preventGrowOverflow={false}>
            <OutcomePieCard pie={view.outcomePie} />
            <SimpleGrid cols={{ base: 1, sm: 2 }} style={{ flex: 1 }}>
              {metrics.map((metric) => (
                <MetricRenderer key={metric.id} metric={metric} />
              ))}
            </SimpleGrid>
          </Group>
        )}

        {summary ? (
          <Stack gap="xs">
            {summary.bullets.map((bullet) => (
              <Text key={bullet}>{bullet}</Text>
            ))}
          </Stack>
        ) : null}
      </Stack>
    </Container>
  );
}
