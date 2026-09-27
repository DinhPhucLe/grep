import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';

import { registerTools } from './tools/register.js';

const SERVER_INSTRUCTIONS = [
  'Cortisol holds organization/team engineering knowledge (attributed notes, gotchas, runbooks, decisions) outside any single repo.',
  'When the user asks about org/team/company knowledge, how “we” do something, search the knowledge base/database, or wants answers that are not in the local codebase, call knowledge_search — do not wait for them to name the tool or say “use MCP”.',
  'Prefer knowledge_search before guessing org-specific behavior or assuming the answer lives only in files you can read.',
  'Use knowledge_post only to persist durable, non-secret insights worth sharing with teammates.',
  'quack is a connectivity smoke test only.',
].join(' ');

export function createCortisolMcpServer(): McpServer {
  const server = new McpServer(
    {
      name: 'cortisol',
      version: '0.1.0',
    },
    {
      instructions: SERVER_INSTRUCTIONS,
      capabilities: {
        logging: {},
      },
    },
  );
  registerTools(server);
  return server;
}
