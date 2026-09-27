import { metricStatuses, type DashboardMetric } from '../contracts/dashboard';
import type {
  ActivityCalendar,
  CalendarDay,
  CodebaseTreemap,
  EmployeePracticeView,
  OrgPracticeView,
  OutcomePie,
  TreemapNode,
} from '../contracts/practice';
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
  // Reuse dashboard metric parser by wrapping a minimal snapshot shell.
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

function parseCalendarDay(raw: unknown, path: string): CalendarDay {
  const day = expectRecord(raw, path);
  return {
    date: expectString(day.date, `${path}.date`),
    count: expectNumber(day.count, `${path}.count`),
    intensity: expectIntensity(day.intensity, `${path}.intensity`),
  };
}

function parseActivityCalendar(raw: unknown, path: string): ActivityCalendar {
  const calendar = expectRecord(raw, path);
  return {
    status: expectOneOf(calendar.status, metricStatuses, `${path}.status`),
    days: expectArray(calendar.days, `${path}.days`).map((day, i) =>
      parseCalendarDay(day, `${path}.days[${i}]`),
    ),
  };
}

function parseOutcomePie(raw: unknown, path: string): OutcomePie {
  const pie = expectRecord(raw, path);
  const display = expectRecord(pie.display, `${path}.display`);
  const secondary = display.secondary;
  if (secondary !== undefined && typeof secondary !== 'string') {
    throw new Error(`${path}.display.secondary must be a string when provided`);
  }
  return {
    status: expectOneOf(pie.status, metricStatuses, `${path}.status`),
    segments: expectArray(pie.segments, `${path}.segments`).map((segment, i) => {
      const value = expectRecord(segment, `${path}.segments[${i}]`);
      return {
        label: expectString(value.label, `${path}.segments[${i}].label`),
        value: expectNumber(value.value, `${path}.segments[${i}].value`),
      };
    }),
    display: {
      primary: expectString(display.primary, `${path}.display.primary`),
      ...(secondary === undefined ? {} : { secondary }),
    },
  };
}

function parseTreemapNode(raw: unknown, path: string): TreemapNode {
  const node = expectRecord(raw, path);
  const children = node.children;
  return {
    name: expectString(node.name, `${path}.name`),
    value: expectNumber(node.value, `${path}.value`),
    intensity: expectIntensity(node.intensity, `${path}.intensity`),
    ...(children === undefined
      ? {}
      : {
          children: expectArray(children, `${path}.children`).map((child, i) =>
            parseTreemapNode(child, `${path}.children[${i}]`),
          ),
        }),
  };
}

function parseCodebaseTreemap(raw: unknown, path: string): CodebaseTreemap {
  const tree = expectRecord(raw, path);
  return {
    projectId: expectString(tree.projectId, `${path}.projectId`),
    repoName: expectString(tree.repoName, `${path}.repoName`),
    status: expectOneOf(tree.status, metricStatuses, `${path}.status`),
    root: parseTreemapNode(tree.root, `${path}.root`),
  };
}

export function parseEmployeePracticeView(raw: unknown): EmployeePracticeView {
  const view = expectRecord(raw, 'employeePractice');
  if (view.schemaVersion !== 'employee_practice.v1') {
    throw new Error('employeePractice.schemaVersion must be employee_practice.v1');
  }
  const subject = expectRecord(view.subject, 'employeePractice.subject');
  const summary = view.summary;
  return {
    schemaVersion: 'employee_practice.v1',
    generatedAt: expectString(view.generatedAt, 'employeePractice.generatedAt'),
    subject: {
      userId: expectString(subject.userId, 'employeePractice.subject.userId'),
      organizationId: expectString(
        subject.organizationId,
        'employeePractice.subject.organizationId',
      ),
      practice: expectString(subject.practice, 'employeePractice.subject.practice'),
      year: expectNumber(subject.year, 'employeePractice.subject.year'),
    },
    activityCalendar: parseActivityCalendar(
      view.activityCalendar,
      'employeePractice.activityCalendar',
    ),
    outcomePie: parseOutcomePie(view.outcomePie, 'employeePractice.outcomePie'),
    metrics: parseMetrics(view.metrics, 'employeePractice.metrics'),
    ...(summary === undefined
      ? {}
      : {
          summary: {
            status: expectOneOf(
              expectRecord(summary, 'employeePractice.summary').status,
              ['available', 'unknown'] as const,
              'employeePractice.summary.status',
            ),
            bullets: expectArray(
              expectRecord(summary, 'employeePractice.summary').bullets,
              'employeePractice.summary.bullets',
            ).map((bullet, i) =>
              expectString(bullet, `employeePractice.summary.bullets[${i}]`),
            ),
          },
        }),
  };
}

export function parseOrgPracticeView(raw: unknown): OrgPracticeView {
  const view = expectRecord(raw, 'orgPractice');
  if (view.schemaVersion !== 'org_practice.v1') {
    throw new Error('orgPractice.schemaVersion must be org_practice.v1');
  }
  const subject = expectRecord(view.subject, 'orgPractice.subject');
  const timeseries = expectRecord(view.timeseries, 'orgPractice.timeseries');
  const summary = view.summary;
  return {
    schemaVersion: 'org_practice.v1',
    generatedAt: expectString(view.generatedAt, 'orgPractice.generatedAt'),
    subject: {
      organizationId: expectString(
        subject.organizationId,
        'orgPractice.subject.organizationId',
      ),
      practice: expectString(subject.practice, 'orgPractice.subject.practice'),
      from: expectString(subject.from, 'orgPractice.subject.from'),
      to: expectString(subject.to, 'orgPractice.subject.to'),
    },
    codebaseTreemaps: expectArray(
      view.codebaseTreemaps,
      'orgPractice.codebaseTreemaps',
    ).map((tree, i) => parseCodebaseTreemap(tree, `orgPractice.codebaseTreemaps[${i}]`)),
    timeseries: {
      status: expectOneOf(timeseries.status, metricStatuses, 'orgPractice.timeseries.status'),
      points: expectArray(timeseries.points, 'orgPractice.timeseries.points').map(
        (point, i) => {
          const value = expectRecord(point, `orgPractice.timeseries.points[${i}]`);
          const median = value.medianActiveAnswerTimeMs;
          if (median !== null && typeof median !== 'number') {
            throw new Error(
              `orgPractice.timeseries.points[${i}].medianActiveAnswerTimeMs must be number or null`,
            );
          }
          return {
            t: expectString(value.t, `orgPractice.timeseries.points[${i}].t`),
            correct: expectNumber(value.correct, `orgPractice.timeseries.points[${i}].correct`),
            failedReveal: expectNumber(
              value.failedReveal,
              `orgPractice.timeseries.points[${i}].failedReveal`,
            ),
            medianActiveAnswerTimeMs: median,
          };
        },
      ),
    },
    metrics: parseMetrics(view.metrics, 'orgPractice.metrics'),
    ...(summary === undefined
      ? {}
      : {
          summary: {
            status: expectOneOf(
              expectRecord(summary, 'orgPractice.summary').status,
              ['available', 'unknown'] as const,
              'orgPractice.summary.status',
            ),
            bullets: expectArray(
              expectRecord(summary, 'orgPractice.summary').bullets,
              'orgPractice.summary.bullets',
            ).map((bullet, i) => expectString(bullet, `orgPractice.summary.bullets[${i}]`)),
          },
        }),
  };
}
