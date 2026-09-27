/** Shared knowledge document shape for MCP tools and the future Go API. */

export type KnowledgeAuthor = {
  userId: string;
  name?: string;
};

export type KnowledgeDocument = {
  id: string;
  content: string;
  topics: string[];
  properties: Record<string, string>;
  authors: KnowledgeAuthor[];
  createdAt: string;
  updatedAt: string;
  organizationId: string;
};

export type KnowledgeSearchResult = {
  items: KnowledgeDocument[];
  scores?: number[];
};
