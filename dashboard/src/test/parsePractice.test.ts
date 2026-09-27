import { describe, expect, it } from 'vitest';
import { parseEmployeePracticeView, parseOrgPracticeView } from '../data/parsePractice';
import employeeMock from '../mocks/employeePractice.mock.json';
import orgMock from '../mocks/orgPractice.mock.json';

describe('parseEmployeePracticeView', () => {
  it('accepts a valid employee practice view', () => {
    const view = parseEmployeePracticeView(employeeMock);
    expect(view.schemaVersion).toBe('employee_practice.v1');
    expect(view.subject.userName).toBe('Alex Rivera');
    expect(view.activityCalendar.days.length).toBeGreaterThan(0);
    expect(view.outcomePie.segments.map((s) => s.label)).toEqual([
      'correct',
      'failed_reveal',
    ]);
  });

  it('rejects the wrong schema version', () => {
    expect(() =>
      parseEmployeePracticeView({ ...employeeMock, schemaVersion: 'dashboard.v1' }),
    ).toThrow(/employee_practice.v1/);
  });
});

describe('parseOrgPracticeView', () => {
  it('accepts a valid org practice view', () => {
    const view = parseOrgPracticeView(orgMock);
    expect(view.schemaVersion).toBe('org_practice.v1');
    expect(view.subject.organizationName).toBe('NovaPay');
    expect(view.codebaseTreemaps.length).toBeGreaterThan(0);
    expect(view.timeseries.points.length).toBeGreaterThan(0);
  });

  it('rejects invalid treemap intensity', () => {
    const bad = structuredClone(orgMock);
    bad.codebaseTreemaps[0].root.intensity = 9;
    expect(() => parseOrgPracticeView(bad)).toThrow(/intensity/);
  });
});
