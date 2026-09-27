import { render, screen } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { describe, expect, it } from 'vitest';
import { EmployeePracticePage } from '../components/practice/EmployeePracticePage';
import { dashboardTheme } from '../config/theme';
import { parseEmployeePracticeView } from '../data/parsePractice';
import employeeMock from '../mocks/employeePractice.mock.json';

const view = parseEmployeePracticeView(employeeMock);

describe('EmployeePracticePage', () => {
  it('renders practice calendar and outcome labels', () => {
    render(
      <MantineProvider theme={dashboardTheme} defaultColorScheme="light">
        <EmployeePracticePage view={view} />
      </MantineProvider>,
    );
    expect(screen.getByRole('heading', { name: 'lead_and_reveal' })).toBeInTheDocument();
    expect(screen.getByLabelText(/Practice activity calendar/)).toBeInTheDocument();
    expect(screen.getByText('Practice instances')).toBeInTheDocument();
  });
});
