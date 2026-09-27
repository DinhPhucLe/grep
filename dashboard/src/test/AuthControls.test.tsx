import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { AuthProvider } from '../auth/AuthProvider';
import { AuthControls } from '../components/AuthControls';
import { dashboardTheme } from '../config/theme';
import { storeSession } from '../auth/session';

function renderAuth() {
  return render(
    <MantineProvider theme={dashboardTheme} defaultColorScheme="light">
      <AuthProvider>
        <AuthControls />
      </AuthProvider>
    </MantineProvider>,
  );
}

describe('AuthControls', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it('shows Sign in when logged out', async () => {
    vi.stubGlobal('fetch', vi.fn());
    renderAuth();
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Sign in/i })).toBeInTheDocument();
    });
  });

  it('shows avatar menu and logs out', async () => {
    storeSession({
      token: 'tok',
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
      user: {
        id: 'u1',
        name: 'Alex',
        githubLogin: 'alexr',
        avatarUrl: '',
      },
      organization: { id: 'o1', name: 'NovaPay' },
    });
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.includes('/api/v1/auth/me')) {
          return new Response(
            JSON.stringify({
              user: { id: 'u1', name: 'Alex', githubLogin: 'alexr' },
              organization: { id: 'o1', name: 'NovaPay' },
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          );
        }
        if (url.includes('/api/v1/auth/logout')) {
          expect(init?.method).toBe('POST');
          return new Response(null, { status: 204 });
        }
        return new Response('nope', { status: 404 });
      }),
    );

    renderAuth();
    const avatar = await screen.findByLabelText(/Account menu for alexr/i);
    fireEvent.click(avatar);
    const logout = await screen.findByText('Log out');
    fireEvent.click(logout);
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Sign in/i })).toBeInTheDocument();
    });
  });

  it('starts device flow from Sign in', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('/api/v1/auth/github/device')) {
          return new Response(
            JSON.stringify({
              deviceCode: 'dev',
              userCode: 'ABCD-EFGH',
              verificationUri: 'https://github.com/login/device',
              expiresIn: 900,
              interval: 5,
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          );
        }
        if (url.includes('/api/v1/auth/github/poll')) {
          return new Response(
            JSON.stringify({
              error: { code: 'authorization_pending', message: 'waiting' },
            }),
            { status: 202, headers: { 'Content-Type': 'application/json' } },
          );
        }
        return new Response('nope', { status: 404 });
      }),
    );

    renderAuth();
    fireEvent.click(await screen.findByRole('button', { name: /Sign in/i }));
    await waitFor(() => {
      expect(screen.getByText('ABCD-EFGH')).toBeInTheDocument();
    });
    expect(screen.getByText('https://github.com/login/device')).toBeInTheDocument();
  });
});
