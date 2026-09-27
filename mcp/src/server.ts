import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';

import { registerTools } from './tools/register.js';

export function createCortisolMcpServer(): McpServer {
  const server = new McpServer(
    {
      name: 'cortisol',
      version: '0.1.0',
    },
    {
      capabilities: {
        logging: {},
      },
    },
  );
  registerTools(server);
  return server;
}
