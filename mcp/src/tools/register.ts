import { z } from 'zod';

import { httpKnowledgePost, httpKnowledgeSearch } from '../http/knowledge.js';
import { mockKnowledgePost, mockKnowledgeSearch } from '../mocks/knowledge.js';
import type { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';

function asText(payload: unknown) {
  return {
    content: [{ type: 'text' as const, text: JSON.stringify(payload, null, 2) }],
  };
}

function backendConfig() {
  return {
    apiBase: process.env.CORTISOL_API_BASE?.trim() || 'http://127.0.0.1:8080',
    orgId: process.env.CORTISOL_ORG_ID?.trim() || 'demo-org',
    sessionToken: process.env.CORTISOL_SESSION_TOKEN?.trim() || undefined,
  };
}

function useHttpBackend(): boolean {
  return (process.env.MCP_BACKEND?.trim() || 'mock') === 'http';
}

const QUACK_MESSAGE = 'this mcp tool work quack quack quack';

export function registerTools(server: McpServer): void {
  server.registerTool(
    'quack',
    {
      title: 'MCP smoke test',
      description:
        'Connectivity smoke test for the Cortisol MCP server. Call this to verify the agent can reach Cortisol tools. Always returns the fixed quack message.',
      inputSchema: {
        note: z
          .string()
          .optional()
          .describe('Optional note ignored by the server; useful for logging on the client side'),
      },
    },
    async () => ({
      content: [{ type: 'text' as const, text: QUACK_MESSAGE }],
    }),
  );

  server.registerTool(
    'knowledge_search',
    {
      title: 'Search org knowledge',
      description: [
        'Search the organization shared knowledge base: attributed teammate notes, gotchas, runbooks, design decisions, and code-locus tips that live outside any one repository.',
        'Call this tool proactively whenever any of these apply (the user does not need to name the tool, say MCP, or say “skill”):',
        '(1) they ask to search the knowledge base/database/org memory;',
        '(2) they want org, team, or company-specific guidance (“how do we…”, “who handled…”, “what’s our convention for…”);',
        '(3) they say the answer is probably not in this codebase, or ask to look outside the repo/codebase/files;',
        '(4) they are stuck on an internal pattern, service, retry/auth/webhook rule, or failure mode another engineer may already have documented.',
        'Prefer this before guessing org-specific behavior or searching only local files when the question is about shared team knowledge.',
        'Pass a natural-language question or a short code/error snippet as query. Returns top-k matches (default 5) with authors and optional scores.',
        'Do not use for general internet facts or public docs. Do not use for Slack history (use Slack MCP). Do not use to write knowledge (use knowledge_post).',
      ].join(' '),
      inputSchema: {
        query: z
          .string()
          .describe(
            'What you need from org memory: a concrete question, symptom, or code/error snippet (e.g. "payment 429 retries", "Stripe webhook HMAC").',
          ),
        k: z.number().int().min(1).max(50).optional().describe('Max results (default 5)'),
        topics: z.array(z.string()).optional().describe('Filter by topic tags'),
        author: z.string().optional().describe('Filter by author userId or name substring'),
        from: z.string().optional().describe('ISO date lower bound on createdAt'),
        to: z.string().optional().describe('ISO date upper bound on createdAt'),
        properties: z
          .record(z.string(), z.string())
          .optional()
          .describe('Exact-match property filters (repo, module, file_path, language, ...)'),
      },
    },
    async (args) => {
      if (useHttpBackend()) {
        return asText(await httpKnowledgeSearch(args, backendConfig()));
      }
      return asText(mockKnowledgeSearch(args));
    },
  );

  server.registerTool(
    'knowledge_post',
    {
      title: 'Post knowledge document',
      description: [
        'Write a durable knowledge card into the organization shared knowledge base so teammates can find it later via knowledge_search.',
        'Call this when the user (or a verified practice outcome) produced a reusable insight: a gotcha, fix, convention, or decision worth attributing.',
        'Include clear content, topic tags, and optional repo/module/file_path properties. Prefer posting only durable, non-secret guidance.',
        'Do not use for transient chat, secrets, or posting to Slack channels (use Slack MCP for Slack).',
      ].join(' '),
      inputSchema: {
        content: z
          .string()
          .min(1)
          .describe('Self-contained knowledge body: the insight, rule, or fix (not a chat transcript)'),
        topics: z.array(z.string()).min(1).describe('Logical topic categories'),
        properties: z
          .record(z.string(), z.string())
          .optional()
          .describe('Identifying key-value properties (repo, module, file_path, ...)'),
        authors: z
          .array(
            z.object({
              userId: z.string(),
              name: z.string().optional(),
            }),
          )
          .optional()
          .describe('People involved with this article'),
      },
    },
    async (args) => {
      if (useHttpBackend()) {
        return asText(await httpKnowledgePost(args, backendConfig()));
      }
      return asText(mockKnowledgePost(args));
    },
  );
}
