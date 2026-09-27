import { render, screen } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { describe, expect, it } from 'vitest';
import { EmployeePracticePage } from '../components/practice/EmployeePracticePage';
import { dashboardTheme } from '../config/theme';
import { parseEmployeeKnowledgeView } from '../data/parseKnowledge';
import { parseEmployeePracticeView } from '../data/parsePractice';
import employeeKnowledgeMock from '../mocks/employeeKnowledge.mock.json';
import employeeMock from '../mocks/employeePractice.mock.json';

const view = parseEmployeePracticeView(employeeMock);
const knowledge = parseEmployeeKnowledgeView(employeeKnowledgeMock);

describe('EmployeePracticePage', () => {
  it('renders practice calendar and outcome labels', () => {
    render(
      <MantineProvider theme={dashboardTheme} defaultColorScheme="light">
        <EmployeePracticePage view={view} knowledge={knowledge} />
      </MantineProvider>,
    );
    expect(screen.getByRole('heading', { name: 'lead_and_reveal' })).toBeInTheDocument();
    expect(screen.getByLabelText(/Practice activity calendar/)).toBeInTheDocument();
    expect(screen.getByText('Practice instances')).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Quizzes' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Edges' })).toBeInTheDocument();
  });
});
