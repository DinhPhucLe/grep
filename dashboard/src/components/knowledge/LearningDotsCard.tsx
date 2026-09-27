import { Paper, Table, Text, Title } from '@mantine/core';
import type { LearningDot, LearningDots } from '../../contracts/knowledge';
import { formatReadableDate, truncateId } from '../../utils/formatPractice';
import { practiceIntensityColor } from '../../utils/practiceIntensity';

function topicTone(topic: string, catalog: string[]): number {
  const idx = catalog.indexOf(topic);
  if (idx < 0) return 1;
  return Math.min(4, Math.max(1, (idx % 4) + 1));
}

function TopicMarks({ topics, catalog }: { topics: string[]; catalog: string[] }) {
  if (topics.length === 0) {
    return <Text className="secondary-text">—</Text>;
  }
  return (
    <div className="learning-topic-row">
      {topics.map((topic) => (
        <span
          key={topic}
          className="learning-topic-chip"
          style={{ background: practiceIntensityColor(topicTone(topic, catalog)) }}
        >
          {topic}
        </span>
      ))}
    </div>
  );
}

function sortDots(items: LearningDot[]): LearningDot[] {
  return [...items].sort((a, b) => b.createdAt.localeCompare(a.createdAt));
}

export function LearningDotsCard({ dots }: { dots: LearningDots }) {
  const items = dots.status === 'available' ? sortDots(dots.items) : [];
  const topicCatalog = [...new Set(items.flatMap((item) => item.topics))];

  return (
    <Paper className="heatmap-panel" p="md" radius={0} withBorder>
      <Text className="eyebrow">Learnings</Text>
      <Title order={3} fz="lg" mt={4} mb="md" className="section-title">
        Connected documents
      </Title>

      {items.length === 0 ? (
        <Text className="secondary-text">No connected documents</Text>
      ) : (
        <Table
          withTableBorder
          withColumnBorders
          highlightOnHover={false}
          className="learning-docs-table"
          aria-label="Connected knowledge documents"
        >
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Doc</Table.Th>
              <Table.Th>Topics</Table.Th>
              <Table.Th>Author</Table.Th>
              <Table.Th>Connected</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {items.map((item) => (
              <Table.Tr key={item.id}>
                <Table.Td>
                  <Text fw={700} ff="monospace" fz="sm">
                    {truncateId(item.id)}
                  </Text>
                </Table.Td>
                <Table.Td>
                  <TopicMarks topics={item.topics} catalog={topicCatalog} />
                </Table.Td>
                <Table.Td>
                  <Text fz="sm" fw={700}>
                    {item.authorName?.trim() || truncateId(item.authorUserId)}
                  </Text>
                </Table.Td>
                <Table.Td>
                  <Text fz="sm">{formatReadableDate(item.createdAt)}</Text>
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      )}
    </Paper>
  );
}
