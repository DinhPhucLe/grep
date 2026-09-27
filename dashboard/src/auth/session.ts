const STORAGE_KEY = 'cortisol.dashboard.session';

export type AuthUser = {
  id: string;
  name: string;
  mail?: string;
  githubLogin?: string;
  avatarUrl?: string;
};

export type AuthOrganization = {
  id: string;
  name: string;
};

export type AuthSession = {
  token: string;
  expiresAt: string;
  user: AuthUser;
  organization: AuthOrganization;
};

function apiBase(): string {
  return (import.meta.env.VITE_API_BASE as string | undefined) ?? '';
}

export function loadStoredSession(): AuthSession | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return null;
    }
    const parsed = JSON.parse(raw) as AuthSession;
    if (!parsed?.token || !parsed?.user?.id) {
      return null;
    }
    if (parsed.expiresAt && Date.parse(parsed.expiresAt) < Date.now()) {
      localStorage.removeItem(STORAGE_KEY);
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

export function storeSession(session: AuthSession): void {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(session));
}

export function clearStoredSession(): void {
  localStorage.removeItem(STORAGE_KEY);
}

export type DeviceStart = {
  deviceCode: string;
  userCode: string;
  verificationUri: string;
  expiresIn: number;
  interval: number;
};

async function readAuthResponse<T>(response: Response): Promise<T> {
  try {
    return (await response.json()) as T;
  } catch {
    if (response.status === 404) {
      throw new Error('Sign-in is unavailable on the running server. Restart the API with the latest code and try again.');
    }
    throw new Error(`The sign-in service returned an unexpected response (HTTP ${response.status}). Please try again.`);
  }
}

export async function startGitHubDevice(): Promise<DeviceStart> {
  const response = await fetch(`${apiBase()}/api/v1/auth/github/device`, {
    method: 'POST',
    headers: { Accept: 'application/json' },
  });
  const body = await readAuthResponse<DeviceStart & {
    error?: { message?: string };
  }>(response);
  if (!response.ok) {
    throw new Error(body.error?.message ?? `device start failed (${response.status})`);
  }
  if (!body.deviceCode || !body.userCode || !body.verificationUri) {
    throw new Error('The sign-in service did not return a verification code. Please try again.');
  }
  return body;
}

export type PollResult =
  | { status: 'pending'; code: string }
  | { status: 'ready'; session: AuthSession }
  | { status: 'error'; message: string };

type PollBody = {
  error?: { code?: string; message?: string };
  token?: string;
  expiresAt?: string;
  user?: AuthUser;
  organization?: AuthOrganization;
};

export async function pollGitHubDevice(deviceCode: string): Promise<PollResult> {
  const response = await fetch(`${apiBase()}/api/v1/auth/github/poll`, {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ deviceCode }),
  });
  const body = await readAuthResponse<PollBody>(response);

  if (response.status === 202) {
    return { status: 'pending', code: body.error?.code ?? 'authorization_pending' };
  }
  if (!response.ok) {
    return {
      status: 'error',
      message: body.error?.message ?? `poll failed (${response.status})`,
    };
  }
  if (!body.token || !body.user?.id || !body.organization?.id) {
    return { status: 'error', message: 'incomplete session response' };
  }
  return {
    status: 'ready',
    session: {
      token: body.token,
      expiresAt: body.expiresAt ?? '',
      user: {
        id: body.user.id,
        name: body.user.name,
        mail: body.user.mail,
        githubLogin: body.user.githubLogin,
        avatarUrl: body.user.avatarUrl,
      },
      organization: {
        id: body.organization.id,
        name: body.organization.name,
      },
    },
  };
}

export async function fetchMe(token: string): Promise<AuthSession | null> {
  const response = await fetch(`${apiBase()}/api/v1/auth/me`, {
    headers: {
      Accept: 'application/json',
      Authorization: `Bearer ${token}`,
    },
  });
  if (response.status === 401) {
    return null;
  }
  if (!response.ok) {
    throw new Error(`auth/me failed (${response.status})`);
  }
  const body = (await response.json()) as {
    expiresAt?: string;
    user: AuthUser;
    organization: AuthOrganization;
  };
  const existing = loadStoredSession();
  return {
    token,
    expiresAt: body.expiresAt ?? existing?.expiresAt ?? '',
    user: body.user,
    organization: body.organization,
  };
}

export async function approveTerminal(id: string, token: string): Promise<void> {
  const response = await fetch(`${apiBase()}/api/v1/auth/cli/approve`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
    body: JSON.stringify({ id }),
  });
  if (!response.ok) {
    const body = await readAuthResponse<{ error?: { message?: string } }>(response);
    throw new Error(body.error?.message ?? 'Could not connect the terminal. Please try again.');
  }
}

export async function logoutSession(token: string): Promise<void> {
  const response = await fetch(`${apiBase()}/api/v1/auth/logout`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!response.ok && response.status !== 401) {
    throw new Error('Could not sign out. Please try again so your terminal is signed out too.');
  }
  clearStoredSession();
}
