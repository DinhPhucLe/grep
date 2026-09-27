import { Badge, Container, Group, Paper, SimpleGrid, Stack, Text, Title } from '@mantine/core';
import { BarChart } from '@mantine/charts';
import { dashboardLayout } from '../../config/layout';
import type { OrgKnowledgeSummary } from '../../contracts/knowledge';
import { formatReadableDate, truncateId } from '../../utils/formatPractice';

function subtitle(summary: OrgKnowledgeSummary): string {
  const idPart = truncateId(summary.organizationId);
  if (summary.organizationName) {
    return `${summary.organizationName} · ${idPart} · ${summary.totalDocuments} knowledge cards`;
  }
  return `Org ${idPart} · ${summary.totalDocuments} knowledge cards`;
}

export function OrgKnowledgePage({ summary }: { summary: OrgKnowledgeSummary }) {
  const topicData = summary.topicCounts.slice(0, 12).map((t) => ({
    topic: t.topic,
    count: t.count,
  }));
  const authorData = summary.authorCounts.slice(0, 10).map((a) => ({
    author: a.name || truncateId(a.userId),
    count: a.count,
  }));

  return (
    <Container
      component="main"
      size={dashboardLayout.maxWidth}
      px={dashboardLayout.pagePadding}
      py={{ base: 'md', sm: 'xl' }}
    >
      <Stack gap={dashboardLayout.sectionGap}>
        <div>
          <Text className="eyebrow">Organization knowledge</Text>
          <Title order={1} className="section-title">
            Shared eng memory
          </Title>
          <Text className="secondary-text">{subtitle(summary)}</Text>
          <Text className="secondary-text" mt="xs">
            Corpus mapped from Snowflake Marketplace / synthetic GitHub export. Vectors via Atlas
            voyage-code-4 autoEmbed (not stored on documents).
          </Text>
        </div>

        <SimpleGrid cols={{ base: 1, md: 3 }}>
          <Paper withBorder p="md" radius={0}>
            <Text className="eyebrow">Documents</Text>
            <Title order={2}>{summary.totalDocuments}</Title>
          </Paper>
          <Paper withBorder p="md" radius={0}>
            <Text className="eyebrow">Topics</Text>
            <Title order={2}>{summary.topicCounts.length}</Title>
          </Paper>
          <Paper withBorder p="md" radius={0}>
            <Text className="eyebrow">Authors</Text>
            <Title order={2}>{summary.authorCounts.length}</Title>
          </Paper>
        </SimpleGrid>

        <SimpleGrid cols={{ base: 1, md: 2 }}>
          <Paper withBorder p="md" radius={0}>
            <Text className="eyebrow" mb="sm">
              Topics
            </Text>
            {topicData.length === 0 ? (
              <Text className="secondary-text">No topics yet — run knowledge seed.</Text>
            ) : (
              <BarChart
                h={280}
                data={topicData}
                dataKey="topic"
                series={[{ name: 'count', color: 'teal.6' }]}
                tickLine="y"
              />
            )}
          </Paper>
          <Paper withBorder p="md" radius={0}>
            <Text className="eyebrow" mb="sm">
              Authors
            </Text>
            {authorData.length === 0 ? (
              <Text className="secondary-text">No authors yet — run knowledge seed.</Text>
            ) : (
              <BarChart
                h={280}
                data={authorData}
                dataKey="author"
                series={[{ name: 'count', color: 'blue.6' }]}
                tickLine="y"
              />
            )}
          </Paper>
        </SimpleGrid>

        <Paper withBorder p="md" radius={0}>
          <Text className="eyebrow" mb="sm">
            Repositories
          </Text>
          <Group gap="xs">
            {summary.repoCounts.length === 0 ? (
              <Text className="secondary-text">No repos tagged.</Text>
            ) : (
              summary.repoCounts.slice(0, 16).map((r) => (
                <Badge key={r.repo} variant="outline" radius={0}>
                  {r.repo} · {r.count}
                </Badge>
              ))
            )}
          </Group>
        </Paper>

        <Paper withBorder p="md" radius={0}>
          <Text className="eyebrow" mb="md">
            Recent cards
          </Text>
          <Stack gap="md">
            {summary.recent.length === 0 ? (
              <Text className="secondary-text">No documents for this organization.</Text>
            ) : (
              summary.recent.map((item) => (
                <Stack key={item.id} gap={4}>
                  <Group gap="sm" justify="space-between">
                    <Text fw={600}>{item.author || 'Unknown author'}</Text>
                    <Text className="secondary-text" size="sm">
                      {formatReadableDate(item.createdAt)}
                    </Text>
                  </Group>
                  <Text>{item.preview}</Text>
                  <Group gap={6}>
                    {item.topics.map((topic) => (
                      <Badge key={topic} size="sm" variant="light" radius={0}>
                        {topic}
                      </Badge>
                    ))}
                  </Group>
                </Stack>
              ))
            )}
          </Stack>
        </Paper>
      </Stack>
    </Container>
  );
}
