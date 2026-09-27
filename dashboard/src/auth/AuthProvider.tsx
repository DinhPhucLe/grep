import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import {
  clearStoredSession,
  fetchMe,
  loadStoredSession,
  logoutSession,
  storeSession,
  type AuthSession,
} from './session';

type AuthContextValue = {
  session: AuthSession | null;
  ready: boolean;
  setSession: (session: AuthSession | null) => void;
  logout: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSessionState] = useState<AuthSession | null>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const stored = loadStoredSession();
    if (!stored) {
      setReady(true);
      return;
    }
    fetchMe(stored.token)
      .then((live) => {
        if (cancelled) {
          return;
        }
        if (!live) {
          clearStoredSession();
          setSessionState(null);
        } else {
          const next = { ...stored, user: live.user, organization: live.organization };
          storeSession(next);
          setSessionState(next);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setSessionState(stored);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setReady(true);
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // The terminal shares this session. Notice logout/revocation there, and sync
  // browser tabs immediately when localStorage changes.
  useEffect(() => {
    if (!session) return;
    let cancelled = false;
    let checking = false;
    const check = async () => {
      if (checking) return;
      checking = true;
      try {
        const live = await fetchMe(session.token);
        if (!cancelled && !live) {
          clearStoredSession();
          setSessionState(null);
        }
      } catch {
        // A temporary network outage does not mean the session was revoked.
      } finally { checking = false; }
    };
    const timer = window.setInterval(() => void check(), 5000);
    window.addEventListener('focus', check);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
      window.removeEventListener('focus', check);
    };
  }, [session?.token]);

  useEffect(() => {
    const sync = (event: StorageEvent) => {
      if (event.key === 'cortisol.dashboard.session' || event.key === null) {
        setSessionState(loadStoredSession());
      }
    };
    window.addEventListener('storage', sync);
    return () => window.removeEventListener('storage', sync);
  }, []);

  const setSession = useCallback((next: AuthSession | null) => {
    if (next) {
      storeSession(next);
    } else {
      clearStoredSession();
    }
    setSessionState(next);
  }, []);

  const logout = useCallback(async () => {
    const token = session?.token;
    if (token) {
      await logoutSession(token);
    }
    setSessionState(null);
    clearStoredSession();
  }, [session?.token]);

  const value = useMemo(
    () => ({ session, ready, setSession, logout }),
    [session, ready, setSession, logout],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error('useAuth requires AuthProvider');
  }
  return ctx;
}
