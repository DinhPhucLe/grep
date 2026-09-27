import { z } from 'zod';

import { mockKnowledgePost, mockKnowledgeSearch } from '../mocks/knowledge.js';
import type { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';

function asText(payload: unknown) {
  return {
    content: [{ type: 'text' as const, text: JSON.stringify(payload, null, 2) }],
  };
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
      description:
        'Return the top-k semantically similar knowledge documents for a query (natural language or code snippet). Mock backend until the Go knowledge API is wired. Default k is 5. For Slack messages/files use the official Slack MCP server, not this tool.',
      inputSchema: {
        query: z.string().describe('Search query: keywords, question, or code snippet'),
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
    async (args) => asText(mockKnowledgeSearch(args)),
  );

  server.registerTool(
    'knowledge_post',
    {
      title: 'Post knowledge document',
      description:
        'Create a knowledge document in the org knowledge base (mock in-memory store). Fields match the shared knowledge contract. To post into Slack channels use the official Slack MCP server.',
      inputSchema: {
        content: z.string().min(1).describe('Knowledge body / article content'),
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
    async (args) => asText(mockKnowledgePost(args)),
  );
}
