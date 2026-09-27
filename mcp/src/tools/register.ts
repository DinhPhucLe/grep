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
        'Search the organization shared knowledge base (attributed notes from teammates: gotchas, runbooks, design decisions, code-locus tips).',
        'Call this when the user is stuck on something another engineer in the org may already have solved, or asks who/how the team handles a pattern, library, service, or failure mode.',
        'Prefer this before guessing org-specific conventions, retry/auth/webhook rules, or internal module behavior.',
        'Pass a natural-language question or a short code/error snippet as query. Returns the top-k matches (default 5) with authors and optional similarity scores.',
        'Do not use for general internet facts, public docs, or Slack history — use Slack MCP for Slack. Do not use for writing new knowledge (use knowledge_post).',
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
