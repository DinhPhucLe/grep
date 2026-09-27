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

export async function startGitHubDevice(): Promise<DeviceStart> {
  const response = await fetch(`${apiBase()}/api/v1/auth/github/device`, {
    method: 'POST',
    headers: { Accept: 'application/json' },
  });
  const body = (await response.json()) as DeviceStart & {
    error?: { message?: string };
  };
  if (!response.ok) {
    throw new Error(body.error?.message ?? `device start failed (${response.status})`);
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
  const body = (await response.json()) as PollBody;

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
    user: AuthUser;
    organization: AuthOrganization;
  };
  const existing = loadStoredSession();
  return {
    token,
    expiresAt: existing?.expiresAt ?? '',
    user: body.user,
    organization: body.organization,
  };
}

export async function logoutSession(token: string): Promise<void> {
  await fetch(`${apiBase()}/api/v1/auth/logout`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
  }).catch(() => undefined);
  clearStoredSession();
}
