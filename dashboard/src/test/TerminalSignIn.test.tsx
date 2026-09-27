import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthProvider } from '../auth/AuthProvider';
import { storeSession, loadStoredSession } from '../auth/session';
import { TerminalSignIn } from '../components/TerminalSignIn';
import { AuthControls } from '../components/AuthControls';

const id = 'a'.repeat(64);
const session = {
  token: 'dashboard-session', expiresAt: new Date(Date.now() + 3600_000).toISOString(),
  user: { id: 'user', name: 'Alex', githubLogin: 'alex' },
  organization: { id: 'org', name: 'NovaPay' },
};

function renderPage() {
  render(<MantineProvider><AuthProvider><AuthControls /><TerminalSignIn /></AuthProvider></MantineProvider>);
}

describe('Dashboard terminal sign-in', () => {
  beforeEach(() => {
    localStorage.clear();
    window.history.replaceState(null, '', `/#terminal=${id}`);
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
    window.history.replaceState(null, '', '/');
  });

  it('requires sign-in before offering terminal approval', async () => {
    const fetch = vi.fn();
    vi.stubGlobal('fetch', fetch);
    renderPage();
    expect(await screen.findByText(/Use Sign in above/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Connect terminal' })).not.toBeInTheDocument();
    expect(fetch).not.toHaveBeenCalled();
  });

  it('shares the session only after explicit approval and clears the handoff URL', async () => {
    storeSession(session);
    const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).endsWith('/auth/me')) return new Response(JSON.stringify(session));
      expect(String(input)).toMatch(/\/auth\/cli\/approve$/);
      expect(init?.headers).toMatchObject({ Authorization: 'Bearer dashboard-session' });
      expect(JSON.parse(String(init?.body))).toEqual({ id });
      return new Response(null, { status: 204 });
    });
    vi.stubGlobal('fetch', fetch);
    renderPage();
    const button = await screen.findByRole('button', { name: 'Connect terminal' });
    expect(fetch).toHaveBeenCalledTimes(1);
    fireEvent.click(button);
    expect(await screen.findByText(/Return to your terminal/)).toBeInTheDocument();
    expect(window.location.hash).toBe('');
  });

  it('shows expired handoffs without claiming success', async () => {
    storeSession(session);
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) =>
      String(input).endsWith('/auth/me') ? new Response(JSON.stringify(session)) :
        new Response(JSON.stringify({ error: { message: 'This terminal sign-in expired.' } }), { status: 410 }),
    ));
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: 'Connect terminal' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('This terminal sign-in expired.');
    expect(screen.queryByText(/Return to your terminal/)).not.toBeInTheDocument();
  });

  it('notices terminal logout when the browser regains focus', async () => {
    storeSession(session);
    let revoked = false;
    vi.stubGlobal('fetch', vi.fn(async () => revoked ? new Response(null, { status: 401 }) : new Response(JSON.stringify(session))));
    renderPage();
    await screen.findByLabelText(/Account menu for alex/);
    revoked = true;
    fireEvent(window, new Event('focus'));
    await screen.findByRole('button', { name: 'Sign in' });
    expect(loadStoredSession()).toBeNull();
    expect(screen.queryByRole('button', { name: 'Connect terminal' })).not.toBeInTheDocument();
  });

  it('keeps the session available to retry when logout fails', async () => {
    storeSession(session);
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) =>
      String(input).endsWith('/auth/me') ? new Response(JSON.stringify(session)) : new Response(null, { status: 503 }),
    ));
    renderPage();
    fireEvent.click(await screen.findByLabelText(/Account menu for alex/));
    fireEvent.click(await screen.findByText('Log out'));
    await waitFor(() => expect(screen.getByText(/Could not sign out/)).toBeInTheDocument());
    expect(loadStoredSession()?.token).toBe('dashboard-session');
  });
});
