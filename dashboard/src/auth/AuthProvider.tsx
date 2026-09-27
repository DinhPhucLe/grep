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
    setSessionState(null);
    clearStoredSession();
    if (token) {
      await logoutSession(token);
    }
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
