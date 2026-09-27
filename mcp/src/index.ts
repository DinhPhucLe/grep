import { createMcpExpressApp } from '@modelcontextprotocol/sdk/server/express.js';
import { StreamableHTTPServerTransport } from '@modelcontextprotocol/sdk/server/streamableHttp.js';

import { createCortisolMcpServer } from './server.js';

function parseListenAddress(raw: string | undefined): { host: string; port: number } {
  const fallback = { host: '0.0.0.0', port: 3100 };
  if (!raw || raw.trim() === '') {
    return fallback;
  }
  const value = raw.trim();
  if (value.startsWith(':')) {
    const port = Number(value.slice(1));
    if (!Number.isFinite(port)) {
      throw new Error(`Invalid MCP_HTTP_ADDR port: ${raw}`);
    }
    return { host: '0.0.0.0', port };
  }
  const idx = value.lastIndexOf(':');
  if (idx <= 0) {
    throw new Error(`Invalid MCP_HTTP_ADDR (expected host:port or :port): ${raw}`);
  }
  const host = value.slice(0, idx);
  const port = Number(value.slice(idx + 1));
  if (!host || !Number.isFinite(port)) {
    throw new Error(`Invalid MCP_HTTP_ADDR: ${raw}`);
  }
  return { host, port };
}

const { host, port } = parseListenAddress(process.env.MCP_HTTP_ADDR);
const app = createMcpExpressApp({ host });

app.get('/health', (_req, res) => {
  res.status(200).json({
    status: 'ok',
    service: 'cortisol-mcp',
    backend: process.env.MCP_BACKEND ?? 'mock',
  });
});

app.post('/mcp', async (req, res) => {
  const server = createCortisolMcpServer();
  try {
    const transport = new StreamableHTTPServerTransport({
      sessionIdGenerator: undefined,
    });
    await server.connect(transport);
    await transport.handleRequest(req, res, req.body);
    res.on('close', () => {
      void transport.close();
      void server.close();
    });
  } catch (error) {
    console.error('Error handling MCP request:', error);
    if (!res.headersSent) {
      res.status(500).json({
        jsonrpc: '2.0',
        error: { code: -32603, message: 'Internal server error' },
        id: null,
      });
    }
  }
});

app.get('/mcp', (_req, res) => {
  res.status(405).json({
    jsonrpc: '2.0',
    error: { code: -32000, message: 'Method not allowed.' },
    id: null,
  });
});

app.delete('/mcp', (_req, res) => {
  res.status(405).json({
    jsonrpc: '2.0',
    error: { code: -32000, message: 'Method not allowed.' },
    id: null,
  });
});

app.listen(port, host, (error?: Error) => {
  if (error) {
    console.error('Failed to start MCP server:', error);
    process.exit(1);
  }
  console.log(
    `cortisol MCP (${process.env.MCP_BACKEND ?? 'mock'} backend) listening on http://${host}:${port}/mcp (health: /health)`,
  );
});

process.on('SIGINT', () => {
  process.exit(0);
});
