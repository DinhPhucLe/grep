import type {
  KnowledgeAuthor,
  KnowledgeDocument,
  KnowledgeSearchResult,
} from '../../contracts/knowledge.js';

export type KnowledgeSearchArgs = {
  query: string;
  k?: number;
  topics?: string[];
  author?: string;
  from?: string;
  to?: string;
  properties?: Record<string, string>;
};

export type KnowledgePostArgs = {
  content: string;
  topics: string[];
  properties?: Record<string, string>;
  authors?: KnowledgeAuthor[];
};

export type HttpBackendConfig = {
  apiBase: string;
  orgId: string;
  sessionToken?: string;
};

function normalizeBase(apiBase: string): string {
  return apiBase.replace(/\/+$/, '');
}

function authHeaders(cfg: HttpBackendConfig, jsonBody: boolean): HeadersInit {
  const headers: Record<string, string> = { Accept: 'application/json' };
  if (jsonBody) {
    headers['Content-Type'] = 'application/json';
  }
  if (cfg.sessionToken) {
    headers.Authorization = `Bearer ${cfg.sessionToken}`;
  }
  return headers;
}

export async function httpKnowledgeSearch(
  args: KnowledgeSearchArgs,
  cfg: HttpBackendConfig,
): Promise<KnowledgeSearchResult> {
  const url = new URL(`${normalizeBase(cfg.apiBase)}/api/v1/knowledge`);
  if (cfg.orgId) {
    url.searchParams.set('organizationId', cfg.orgId);
  }
  url.searchParams.set('query', args.query);
  url.searchParams.set('k', String(args.k ?? 5));
  if (args.author) {
    url.searchParams.set('author', args.author);
  }
  if (args.from) {
    url.searchParams.set('from', args.from);
  }
  if (args.to) {
    url.searchParams.set('to', args.to);
  }
  if (args.topics) {
    for (const t of args.topics) {
      url.searchParams.append('topics', t);
    }
  }
  if (args.properties) {
    for (const [key, value] of Object.entries(args.properties)) {
      url.searchParams.set(`prop.${key}`, value);
    }
  }

  const res = await fetch(url, { method: 'GET', headers: authHeaders(cfg, false) });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`knowledge search failed (${res.status}): ${body}`);
  }
  return (await res.json()) as KnowledgeSearchResult;
}

export async function httpKnowledgePost(
  args: KnowledgePostArgs,
  cfg: HttpBackendConfig,
): Promise<KnowledgeDocument> {
  const payload: Record<string, unknown> = {
    content: args.content,
    topics: args.topics,
    properties: args.properties ?? {},
  };
  // When authenticated, the API derives authors and organizationId from the session.
  if (!cfg.sessionToken) {
    payload.authors = args.authors ?? [{ userId: 'mcp', name: 'MCP' }];
    payload.organizationId = cfg.orgId;
  }

  const res = await fetch(`${normalizeBase(cfg.apiBase)}/api/v1/knowledge`, {
    method: 'POST',
    headers: authHeaders(cfg, true),
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`knowledge post failed (${res.status}): ${body}`);
  }
  return (await res.json()) as KnowledgeDocument;
}
