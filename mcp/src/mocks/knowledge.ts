import { randomUUID } from 'node:crypto';
import { readFileSync } from 'node:fs';
import path from 'node:path';

import type {
  KnowledgeAuthor,
  KnowledgeDocument,
  KnowledgeSearchResult,
} from '../../contracts/knowledge.js';

// Fixtures resolve from the package root (cwd when using npm run / Docker WORKDIR).
const packageRoot = process.env.CORTISOL_MCP_ROOT?.trim() || process.cwd();

function defaultOrgId(): string {
  return process.env.CORTISOL_ORG_ID?.trim() || 'demo-org';
}

function loadSeed(): KnowledgeDocument[] {
  const raw = readFileSync(path.join(packageRoot, 'fixtures', 'knowledge.json'), 'utf8');
  const docs = JSON.parse(raw) as KnowledgeDocument[];
  const org = defaultOrgId();
  return docs.map((d) => ({ ...d, organizationId: org }));
}

let store: KnowledgeDocument[] = loadSeed();

function tokenize(text: string): string[] {
  return text
    .toLowerCase()
    .split(/[^a-z0-9_/.-]+/)
    .filter((t) => t.length > 1);
}

function scoreDoc(query: string, doc: KnowledgeDocument): number {
  const qTokens = new Set(tokenize(query));
  if (qTokens.size === 0) {
    return 0;
  }
  const hay = tokenize(
    [doc.content, ...doc.topics, ...Object.values(doc.properties), ...doc.authors.map((a) => a.name ?? '')].join(' '),
  );
  let hits = 0;
  for (const t of hay) {
    if (qTokens.has(t)) {
      hits += 1;
    }
  }
  return hits / qTokens.size;
}

export type KnowledgeSearchArgs = {
  query: string;
  k?: number;
  topics?: string[];
  author?: string;
  from?: string;
  to?: string;
  properties?: Record<string, string>;
};

export function mockKnowledgeSearch(args: KnowledgeSearchArgs): KnowledgeSearchResult {
  const k = args.k ?? 5;
  const org = defaultOrgId();
  let candidates = store.filter((d) => d.organizationId === org);

  if (args.topics && args.topics.length > 0) {
    const want = new Set(args.topics.map((t) => t.toLowerCase()));
    candidates = candidates.filter((d) => d.topics.some((t) => want.has(t.toLowerCase())));
  }
  if (args.author) {
    const a = args.author.toLowerCase();
    candidates = candidates.filter((d) =>
      d.authors.some((x) => x.userId === args.author || (x.name && x.name.toLowerCase().includes(a))),
    );
  }
  if (args.from) {
    const from = Date.parse(args.from);
    candidates = candidates.filter((d) => Date.parse(d.createdAt) >= from);
  }
  if (args.to) {
    const to = Date.parse(args.to);
    candidates = candidates.filter((d) => Date.parse(d.createdAt) <= to);
  }
  if (args.properties) {
    for (const [key, value] of Object.entries(args.properties)) {
      candidates = candidates.filter((d) => d.properties[key] === value);
    }
  }

  const ranked = candidates
    .map((doc) => ({ doc, score: scoreDoc(args.query, doc) }))
    .filter((r) => r.score > 0)
    .sort((a, b) => b.score - a.score)
    .slice(0, Math.max(1, k));

  // If nothing matched tokens, fall back to most recent within filters.
  if (ranked.length === 0) {
    const fallback = [...candidates]
      .sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt))
      .slice(0, Math.max(1, k));
    return { items: fallback, scores: fallback.map(() => 0) };
  }

  return {
    items: ranked.map((r) => r.doc),
    scores: ranked.map((r) => r.score),
  };
}

export type KnowledgePostArgs = {
  content: string;
  topics: string[];
  properties?: Record<string, string>;
  authors?: KnowledgeAuthor[];
};

export function mockKnowledgePost(args: KnowledgePostArgs): KnowledgeDocument {
  const now = new Date().toISOString();
  const doc: KnowledgeDocument = {
    id: randomUUID().replace(/-/g, '').slice(0, 24),
    content: args.content,
    topics: args.topics,
    properties: args.properties ?? {},
    authors: args.authors ?? [],
    createdAt: now,
    updatedAt: now,
    organizationId: defaultOrgId(),
  };
  store = [doc, ...store];
  return doc;
}

/** Test helper: reset in-memory store from seed fixtures. */
export function resetKnowledgeStore(): void {
  store = loadSeed();
}
