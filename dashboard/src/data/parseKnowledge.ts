import { metricStatuses, type DashboardMetric } from '../contracts/dashboard';
import type {
  EmployeeKnowledgeView,
  LearningDot,
  LearningDots,
  OrgKnowledgeView,
  TopicLink,
  TopicNetwork,
  TopicNode,
} from '../contracts/knowledge';
import { parseDashboardSnapshot } from './parseDashboard';

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

const expectRecord = (value: unknown, path: string): Record<string, unknown> => {
  if (!isRecord(value)) throw new Error(`${path} must be an object`);
  return value;
};

const expectString = (value: unknown, path: string): string => {
  if (typeof value !== 'string') throw new Error(`${path} must be a string`);
  return value;
};

const expectNumber = (value: unknown, path: string): number => {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    throw new Error(`${path} must be a finite number`);
  }
  return value;
};

const expectArray = (value: unknown, path: string): unknown[] => {
  if (!Array.isArray(value)) throw new Error(`${path} must be an array`);
  return value;
};

const expectOneOf = <T extends string>(
  value: unknown,
  options: readonly T[],
  path: string,
): T => {
  if (typeof value !== 'string' || !options.includes(value as T)) {
    throw new Error(`${path} must be one of: ${options.join(', ')}`);
  }
  return value as T;
};

const expectIntensity = (value: unknown, path: string): 0 | 1 | 2 | 3 | 4 => {
  const n = expectNumber(value, path);
  if (![0, 1, 2, 3, 4].includes(n)) {
    throw new Error(`${path} intensity must be an integer from 0 through 4`);
  }
  return n as 0 | 1 | 2 | 3 | 4;
};

function parseMetrics(raw: unknown, path: string): DashboardMetric[] {
  const wrapped = parseDashboardSnapshot({
    schemaVersion: 'dashboard.v1',
    sequence: 1,
    generatedAt: '1970-01-01T00:00:00Z',
    session: {
      id: 'parse-helper',
      startedAt: '1970-01-01T00:00:00Z',
      observedThrough: '1970-01-01T00:00:00Z',
      state: 'complete',
    },
    metrics: expectArray(raw, path),
    summary: { status: 'unknown', bullets: [] },
    heatmap: { mode: 'final_diff', files: [] },
  });
  return wrapped.metrics;
}

function parseActivityCalendar(raw: unknown, path: string) {
  const calendar = expectRecord(raw, path);
  return {
    status: expectOneOf(calendar.status, metricStatuses, `${path}.status`),
    days: expectArray(calendar.days, `${path}.days`).map((day, i) => {
      const value = expectRecord(day, `${path}.days[${i}]`);
      return {
        date: expectString(value.date, `${path}.days[${i}].date`),
        count: expectNumber(value.count, `${path}.days[${i}].count`),
        intensity: expectIntensity(value.intensity, `${path}.days[${i}].intensity`),
      };
    }),
  };
}

function parseSummary(raw: unknown, path: string) {
  const summary = expectRecord(raw, path);
  return {
    status: expectOneOf(summary.status, ['available', 'unknown'] as const, `${path}.status`),
    bullets: expectArray(summary.bullets, `${path}.bullets`).map((bullet, i) =>
      expectString(bullet, `${path}.bullets[${i}]`),
    ),
  };
}

function parseTopicNode(raw: unknown, path: string): TopicNode {
  const node = expectRecord(raw, path);
  return {
    id: expectString(node.id, `${path}.id`),
    label: expectString(node.label, `${path}.label`),
    weight: expectNumber(node.weight, `${path}.weight`),
  };
}

function parseTopicLink(raw: unknown, path: string): TopicLink {
  const link = expectRecord(raw, path);
  return {
    source: expectString(link.source, `${path}.source`),
    target: expectString(link.target, `${path}.target`),
    weight: expectNumber(link.weight, `${path}.weight`),
  };
}

function parseTopicNetwork(raw: unknown, path: string): TopicNetwork {
  const network = expectRecord(raw, path);
  return {
    status: expectOneOf(network.status, metricStatuses, `${path}.status`),
    nodes: expectArray(network.nodes, `${path}.nodes`).map((node, i) =>
      parseTopicNode(node, `${path}.nodes[${i}]`),
    ),
    links: expectArray(network.links, `${path}.links`).map((link, i) =>
      parseTopicLink(link, `${path}.links[${i}]`),
    ),
  };
}

function parseLearningDot(raw: unknown, path: string): LearningDot {
  const item = expectRecord(raw, path);
  return {
    id: expectString(item.id, `${path}.id`),
    topics: expectArray(item.topics, `${path}.topics`).map((topic, i) =>
      expectString(topic, `${path}.topics[${i}]`),
    ),
    authorUserId: expectString(item.authorUserId, `${path}.authorUserId`),
    ...(item.authorName === undefined
      ? {}
      : { authorName: expectString(item.authorName, `${path}.authorName`) }),
    createdAt: expectString(item.createdAt, `${path}.createdAt`),
  };
}

function parseLearningDots(raw: unknown, path: string): LearningDots {
  const dots = expectRecord(raw, path);
  return {
    status: expectOneOf(dots.status, metricStatuses, `${path}.status`),
    items: expectArray(dots.items, `${path}.items`).map((item, i) =>
      parseLearningDot(item, `${path}.items[${i}]`),
    ),
  };
}

export function parseEmployeeKnowledgeView(raw: unknown): EmployeeKnowledgeView {
  const view = expectRecord(raw, 'employeeKnowledge');
  if (view.schemaVersion !== 'employee_knowledge.v1') {
    throw new Error('employeeKnowledge.schemaVersion must be employee_knowledge.v1');
  }
  const subject = expectRecord(view.subject, 'employeeKnowledge.subject');
  const summary = view.summary;
  return {
    schemaVersion: 'employee_knowledge.v1',
    generatedAt: expectString(view.generatedAt, 'employeeKnowledge.generatedAt'),
    subject: {
      userId: expectString(subject.userId, 'employeeKnowledge.subject.userId'),
      ...(subject.userName === undefined
        ? {}
        : {
            userName: expectString(subject.userName, 'employeeKnowledge.subject.userName'),
          }),
      ...(subject.organizationId === undefined
        ? {}
        : {
            organizationId: expectString(
              subject.organizationId,
              'employeeKnowledge.subject.organizationId',
            ),
          }),
      year: expectNumber(subject.year, 'employeeKnowledge.subject.year'),
    },
    activityCalendar: parseActivityCalendar(
      view.activityCalendar,
      'employeeKnowledge.activityCalendar',
    ),
    metrics: parseMetrics(view.metrics, 'employeeKnowledge.metrics'),
    ...(summary === undefined
      ? {}
      : { summary: parseSummary(summary, 'employeeKnowledge.summary') }),
  };
}

export function parseOrgKnowledgeView(raw: unknown): OrgKnowledgeView {
  const view = expectRecord(raw, 'orgKnowledge');
  if (view.schemaVersion !== 'org_knowledge.v1') {
    throw new Error('orgKnowledge.schemaVersion must be org_knowledge.v1');
  }
  const subject = expectRecord(view.subject, 'orgKnowledge.subject');
  const summary = view.summary;
  return {
    schemaVersion: 'org_knowledge.v1',
    generatedAt: expectString(view.generatedAt, 'orgKnowledge.generatedAt'),
    subject: {
      organizationId: expectString(
        subject.organizationId,
        'orgKnowledge.subject.organizationId',
      ),
      ...(subject.organizationName === undefined
        ? {}
        : {
            organizationName: expectString(
              subject.organizationName,
              'orgKnowledge.subject.organizationName',
            ),
          }),
      from: expectString(subject.from, 'orgKnowledge.subject.from'),
      to: expectString(subject.to, 'orgKnowledge.subject.to'),
    },
    topicNetwork: parseTopicNetwork(view.topicNetwork, 'orgKnowledge.topicNetwork'),
    learningDots: parseLearningDots(view.learningDots, 'orgKnowledge.learningDots'),
    metrics: parseMetrics(view.metrics, 'orgKnowledge.metrics'),
    ...(summary === undefined ? {} : { summary: parseSummary(summary, 'orgKnowledge.summary') }),
  };
}
