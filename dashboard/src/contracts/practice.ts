import type { DashboardMetric, MetricStatus } from './dashboard';

export type PracticeStatus = MetricStatus;

export interface CalendarDay {
  date: string;
  count: number;
  intensity: 0 | 1 | 2 | 3 | 4;
}

export interface ActivityCalendar {
  status: PracticeStatus;
  days: CalendarDay[];
}

export interface PieSegment {
  label: string;
  value: number;
}

export interface OutcomePie {
  status: PracticeStatus;
  segments: PieSegment[];
  display: { primary: string; secondary?: string };
}

export interface EmployeeSubject {
  userId: string;
  userName?: string;
  organizationId: string;
  practice: string;
  year: number;
}

export interface EmployeePracticeView {
  schemaVersion: 'employee_practice.v1';
  generatedAt: string;
  subject: EmployeeSubject;
  activityCalendar: ActivityCalendar;
  outcomePie: OutcomePie;
  metrics: DashboardMetric[];
  summary?: { status: 'available' | 'unknown'; bullets: string[] };
}

export interface TreemapNode {
  name: string;
  value: number;
  intensity: 0 | 1 | 2 | 3 | 4;
  children?: TreemapNode[];
}

export interface CodebaseTreemap {
  projectId: string;
  repoName: string;
  status: PracticeStatus;
  root: TreemapNode;
}

export interface TimeseriesPoint {
  t: string;
  correct: number;
  failedReveal: number;
  medianActiveAnswerTimeMs: number | null;
}

export interface OrgSubject {
  organizationId: string;
  organizationName?: string;
  practice: string;
  from: string;
  to: string;
}

export interface OrgPracticeView {
  schemaVersion: 'org_practice.v1';
  generatedAt: string;
  subject: OrgSubject;
  codebaseTreemaps: CodebaseTreemap[];
  timeseries: {
    status: PracticeStatus;
    points: TimeseriesPoint[];
  };
  metrics: DashboardMetric[];
  summary?: { status: 'available' | 'unknown'; bullets: string[] };
}
