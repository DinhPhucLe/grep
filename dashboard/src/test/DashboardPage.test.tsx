import { fireEvent, render, screen } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { describe, expect, it } from 'vitest';
import mockDashboard from '../mocks/dashboard.mock.json';
import { DashboardPage } from '../components/DashboardPage';
import { dashboardTheme } from '../config/theme';
import { parseDashboardSnapshot } from '../data/parseDashboard';

const snapshot = parseDashboardSnapshot(mockDashboard);

function renderDashboard() {
  return render(
    <MantineProvider theme={dashboardTheme} defaultColorScheme="light">
      <DashboardPage snapshot={snapshot} status="mock" />
    </MantineProvider>,
  );
}

describe('DashboardPage', () => {
  it('renders all required metrics and summary evidence', () => {
    renderDashboard();

    expect(screen.getByRole('heading', { name: 'Session evidence' })).toBeInTheDocument();
    expect(screen.getByText('Acceptance ratio')).toBeInTheDocument();
    expect(screen.getByText('Median time to approval')).toBeInTheDocument();
    expect(screen.getByText('Median active post-edit response')).toBeInTheDocument();
    expect(screen.getByText('Estimated active time')).toBeInTheDocument();
    expect(screen.getByText('Files edited')).toBeInTheDocument();
    expect(screen.getByText('Final added lines')).toBeInTheDocument();
    expect(screen.getByText('Model requests')).toBeInTheDocument();
    expect(screen.getByText('Distinct plans')).toBeInTheDocument();
    expect(screen.getByText('Acceptances')).toBeInTheDocument();
    expect(screen.getByText('Rejections')).toBeInTheDocument();
    expect(screen.getByText('User prompts')).toBeInTheDocument();
    expect(screen.getByText('Average prompt length')).toBeInTheDocument();
    expect(screen.getByText('Median prompt length')).toBeInTheDocument();
    expect(screen.getByText('Output tokens')).toBeInTheDocument();
    expect(screen.getByText(/Raw CLI event telemetry is not connected yet/)).toBeInTheDocument();
  });

  it('renders ratio context and both histograms', () => {
    renderDashboard();

    expect(screen.getByText('13 of 20 shown')).toBeInTheDocument();
    expect(screen.getByLabelText('Median time to approval distribution')).toBeInTheDocument();
    expect(
      screen.getByLabelText('Median active post-edit response distribution'),
    ).toBeInTheDocument();
  });

  it('changes code rows when a file is selected', () => {
    renderDashboard();

    expect(screen.getByText('func AcceptanceRatio(shown, accepted int) *float64 {')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('tab', { name: 'README.md' }));
    expect(screen.getByText('# Cortisol dashboard')).toBeInTheDocument();
    expect(
      screen.queryByText('func AcceptanceRatio(shown, accepted int) *float64 {'),
    ).not.toBeInTheDocument();
  });
});
