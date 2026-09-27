import assert from 'node:assert/strict';
import { afterEach, describe, it, mock } from 'node:test';

import { httpKnowledgePost, httpKnowledgeSearch } from './knowledge.js';

describe('httpKnowledgeSearch', () => {
  afterEach(() => {
    mock.restoreAll();
  });

  it('GETs /api/v1/knowledge with org id query and k', async () => {
    const calls: { url: string; init?: RequestInit }[] = [];
    mock.method(globalThis, 'fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({ url: String(input), init });
      return new Response(
        JSON.stringify({
          items: [
            {
              id: 'abc',
              content: 'Use exponential backoff with jitter on 429/503.',
              topics: ['payments'],
              properties: {},
              authors: [{ userId: 'alexr' }],
              createdAt: '2025-01-01T00:00:00Z',
              updatedAt: '2025-01-01T00:00:00Z',
              organizationId: 'org-novapay',
            },
          ],
          scores: [0.91],
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    });

    const result = await httpKnowledgeSearch(
      { query: 'payment retry', k: 5 },
      { apiBase: 'http://api:8080', orgId: 'org-novapay', sessionToken: 'tok' },
    );

    assert.equal(calls.length, 1);
    const url = new URL(calls[0].url);
    assert.equal(url.origin + url.pathname, 'http://api:8080/api/v1/knowledge');
    assert.equal(url.searchParams.get('organizationId'), 'org-novapay');
    assert.equal(url.searchParams.get('query'), 'payment retry');
    assert.equal(url.searchParams.get('k'), '5');
    assert.equal((calls[0].init?.headers as Record<string, string>).Authorization, 'Bearer tok');
    assert.equal(result.items.length, 1);
    assert.equal(result.scores?.[0], 0.91);
  });

  it('surfaces non-2xx as error', async () => {
    mock.method(globalThis, 'fetch', async () => new Response('nope', { status: 500 }));
    await assert.rejects(
      () => httpKnowledgeSearch({ query: 'x' }, { apiBase: 'http://api:8080', orgId: 'org' }),
      /knowledge search failed \(500\)/,
    );
  });
});

describe('httpKnowledgePost', () => {
  afterEach(() => {
    mock.restoreAll();
  });

  it('POSTs document without spoofable authors when session token present', async () => {
    const calls: { url: string; init?: RequestInit }[] = [];
    mock.method(globalThis, 'fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({ url: String(input), init });
      return new Response(
        JSON.stringify({
          id: 'newid',
          content: 'note',
          topics: ['t'],
          properties: {},
          authors: [{ userId: 'u' }],
          createdAt: '2025-01-01T00:00:00Z',
          updatedAt: '2025-01-01T00:00:00Z',
          organizationId: 'org-novapay',
        }),
        { status: 201, headers: { 'Content-Type': 'application/json' } },
      );
    });

    const doc = await httpKnowledgePost(
      { content: 'note', topics: ['t'], authors: [{ userId: 'u' }] },
      { apiBase: 'http://api:8080', orgId: 'org-novapay', sessionToken: 'tok' },
    );

    assert.equal(calls.length, 1);
    assert.equal(calls[0].url, 'http://api:8080/api/v1/knowledge');
    assert.equal(calls[0].init?.method, 'POST');
    const body = JSON.parse(String(calls[0].init?.body));
    assert.equal(body.organizationId, undefined);
    assert.equal(body.authors, undefined);
    assert.equal(body.content, 'note');
    assert.equal(doc.id, 'newid');
  });
});
