import { describe, expect, it } from 'vitest';
import { parseOrgKnowledgeSummary } from '../data/parseKnowledge';

describe('parseOrgKnowledgeSummary', () => {
  it('parses a pitch summary payload', () => {
    const summary = parseOrgKnowledgeSummary({
      organizationId: '660100000000000004000000',
      organizationName: 'NovaPay',
      totalDocuments: 2,
      topicCounts: [{ topic: 'payments', count: 2 }],
      authorCounts: [{ userId: 'u1', name: 'Alex Rivera', count: 2 }],
      repoCounts: [{ repo: 'novapay/novapay-api', count: 2 }],
      recent: [
        {
          id: 'abc',
          preview: 'Retry with jitter',
          topics: ['payments'],
          author: 'Alex Rivera',
          createdAt: '2026-01-01T00:00:00Z',
        },
      ],
    });
    expect(summary.totalDocuments).toBe(2);
    expect(summary.topicCounts[0].topic).toBe('payments');
    expect(summary.recent[0].preview).toContain('Retry');
  });
});
