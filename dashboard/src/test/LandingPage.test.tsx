import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { resolveRoutePath } from '../App';
import { LandingPage } from '../components/LandingPage';
import { dashboardTheme } from '../config/theme';

describe('resolveRoutePath', () => {
  it('maps people and organization paths', () => {
    expect(resolveRoutePath('/people/abc')).toEqual({ kind: 'people', id: 'abc' });
    expect(resolveRoutePath('/organizations/org1')).toEqual({
      kind: 'organizations',
      id: 'org1',
    });
    expect(resolveRoutePath('/')).toEqual({ kind: 'landing' });
  });
});

describe('LandingPage', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('/people/recommendations')) {
          return new Response(
            JSON.stringify({
              items: [{ id: 'u1', name: 'user1', mail: 'user1@example.com' }],
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          );
        }
        if (url.includes('/organizations/recommendations')) {
          return new Response(
            JSON.stringify({
              items: [{ id: 'o1', name: 'demo-org' }],
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          );
        }
        return new Response('not found', { status: 404 });
      }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders empty landing with people and organization search', async () => {
    render(
      <MantineProvider theme={dashboardTheme} defaultColorScheme="light">
        <LandingPage />
      </MantineProvider>,
    );

    expect(screen.getByRole('heading', { name: 'Dashboard' })).toBeInTheDocument();
    expect(screen.getByText(/Select a person or organization/i)).toBeInTheDocument();

    fireEvent.focus(screen.getByLabelText(/Search people/i));
    await waitFor(() => {
      expect(screen.getByText('user1')).toBeInTheDocument();
    });
    expect(screen.getByText('user1@example.com')).toBeInTheDocument();
  });
});
