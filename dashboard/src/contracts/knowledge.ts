/** Org knowledge pitch aggregates from GET .../organizations/{id}/knowledge */

export type KnowledgeTopicCount = {
  topic: string;
  count: number;
};

export type KnowledgeAuthorCount = {
  userId: string;
  name?: string;
  count: number;
};

export type KnowledgeRepoCount = {
  repo: string;
  count: number;
};

export type KnowledgeRecentItem = {
  id: string;
  preview: string;
  topics: string[];
  author?: string;
  createdAt: string;
};

export type OrgKnowledgeSummary = {
  organizationId: string;
  organizationName?: string;
  totalDocuments: number;
  topicCounts: KnowledgeTopicCount[];
  authorCounts: KnowledgeAuthorCount[];
  repoCounts: KnowledgeRepoCount[];
  recent: KnowledgeRecentItem[];
};
