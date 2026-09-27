import type { OrgKnowledgeSummary } from '../contracts/knowledge';

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function asString(value: unknown, field: string): string {
  if (typeof value !== 'string') {
    throw new Error(`Invalid knowledge summary: ${field} must be a string`);
  }
  return value;
}

function asNumber(value: unknown, field: string): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    throw new Error(`Invalid knowledge summary: ${field} must be a number`);
  }
  return value;
}

export function parseOrgKnowledgeSummary(raw: unknown): OrgKnowledgeSummary {
  if (!isRecord(raw)) {
    throw new Error('Invalid knowledge summary: expected object');
  }
  const topicCounts = Array.isArray(raw.topicCounts)
    ? raw.topicCounts.map((item, i) => {
        if (!isRecord(item)) {
          throw new Error(`Invalid topicCounts[${i}]`);
        }
        return { topic: asString(item.topic, `topicCounts[${i}].topic`), count: asNumber(item.count, `topicCounts[${i}].count`) };
      })
    : [];
  const authorCounts = Array.isArray(raw.authorCounts)
    ? raw.authorCounts.map((item, i) => {
        if (!isRecord(item)) {
          throw new Error(`Invalid authorCounts[${i}]`);
        }
        return {
          userId: asString(item.userId, `authorCounts[${i}].userId`),
          name: typeof item.name === 'string' ? item.name : undefined,
          count: asNumber(item.count, `authorCounts[${i}].count`),
        };
      })
    : [];
  const repoCounts = Array.isArray(raw.repoCounts)
    ? raw.repoCounts.map((item, i) => {
        if (!isRecord(item)) {
          throw new Error(`Invalid repoCounts[${i}]`);
        }
        return { repo: asString(item.repo, `repoCounts[${i}].repo`), count: asNumber(item.count, `repoCounts[${i}].count`) };
      })
    : [];
  const recent = Array.isArray(raw.recent)
    ? raw.recent.map((item, i) => {
        if (!isRecord(item)) {
          throw new Error(`Invalid recent[${i}]`);
        }
        const topics = Array.isArray(item.topics)
          ? item.topics.filter((t): t is string => typeof t === 'string')
          : [];
        return {
          id: asString(item.id, `recent[${i}].id`),
          preview: asString(item.preview, `recent[${i}].preview`),
          topics,
          author: typeof item.author === 'string' ? item.author : undefined,
          createdAt: asString(item.createdAt, `recent[${i}].createdAt`),
        };
      })
    : [];

  return {
    organizationId: asString(raw.organizationId, 'organizationId'),
    organizationName: typeof raw.organizationName === 'string' ? raw.organizationName : undefined,
    totalDocuments: asNumber(raw.totalDocuments, 'totalDocuments'),
    topicCounts,
    authorCounts,
    repoCounts,
    recent,
  };
}
