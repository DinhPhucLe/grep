import type { DashboardMetric, MetricStatus } from './dashboard';
import type { ActivityCalendar } from './practice';

export interface EmployeeKnowledgeSubject {
  userId: string;
  userName?: string;
  organizationId?: string;
  year: number;
}

export interface EmployeeKnowledgeView {
  schemaVersion: 'employee_knowledge.v1';
  generatedAt: string;
  subject: EmployeeKnowledgeSubject;
  activityCalendar: ActivityCalendar;
  metrics: DashboardMetric[];
  summary?: { status: 'available' | 'unknown'; bullets: string[] };
}

export interface OrgKnowledgeSubject {
  organizationId: string;
  organizationName?: string;
  from: string;
  to: string;
}

export interface TopicNode {
  id: string;
  label: string;
  weight: number;
}

export interface TopicLink {
  source: string;
  target: string;
  weight: number;
}

export interface TopicNetwork {
  status: MetricStatus;
  nodes: TopicNode[];
  links: TopicLink[];
}

export interface LearningDot {
  id: string;
  topics: string[];
  authorUserId: string;
  authorName?: string;
  createdAt: string;
}

export interface LearningDots {
  status: MetricStatus;
  items: LearningDot[];
}

export interface OrgKnowledgeView {
  schemaVersion: 'org_knowledge.v1';
  generatedAt: string;
  subject: OrgKnowledgeSubject;
  topicNetwork: TopicNetwork;
  learningDots: LearningDots;
  metrics: DashboardMetric[];
  summary?: { status: 'available' | 'unknown'; bullets: string[] };
}
