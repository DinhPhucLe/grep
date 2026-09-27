import { describe, expect, it } from 'vitest';
import { parseEmployeeKnowledgeView, parseOrgKnowledgeView } from '../data/parseKnowledge';
import employeeKnowledgeMock from '../mocks/employeeKnowledge.mock.json';
import orgKnowledgeMock from '../mocks/orgKnowledge.mock.json';

describe('parseKnowledge', () => {
  it('parses employee knowledge mock', () => {
    const view = parseEmployeeKnowledgeView(employeeKnowledgeMock);
    expect(view.schemaVersion).toBe('employee_knowledge.v1');
    expect(view.activityCalendar.days.length).toBeGreaterThan(0);
    expect(view.metrics.some((m) => m.id === 'in_out_ratio')).toBe(true);
  });

  it('parses org knowledge mock', () => {
    const view = parseOrgKnowledgeView(orgKnowledgeMock);
    expect(view.schemaVersion).toBe('org_knowledge.v1');
    expect(view.topicNetwork.nodes.length).toBeGreaterThan(0);
    expect(view.learningDots.items.length).toBeGreaterThan(0);
  });
});
