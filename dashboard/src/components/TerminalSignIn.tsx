import { useState } from 'react';
import { Alert, Button, Group, Stack, Text } from '@mantine/core';
import { useAuth } from '../auth/AuthProvider';
import { approveTerminal } from '../auth/session';

export function TerminalSignIn() {
  const { session, ready } = useAuth();
  const [id] = useState(() => new URLSearchParams(window.location.hash.slice(1)).get('terminal'));
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState('');
  if (!id || !/^[a-f0-9]{64}$/.test(id)) return null;

  const connect = async () => {
    if (!session) return;
    setBusy(true);
    setError('');
    try {
      await approveTerminal(id, session.token);
      setDone(true);
      window.history.replaceState(null, '', window.location.pathname + window.location.search);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not connect the terminal.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <Alert title={done ? 'Terminal connected' : 'Sign in to your terminal'} maw={720} mx="auto" mt="xl">
      <Stack gap="sm">
        {done ? <Text>Return to your terminal to continue. Signing out here or in the terminal signs out both.</Text> : <>
          <Text>Connect only the terminal you opened this page from. Its connection code should be {id.slice(0, 8).toUpperCase()}.</Text>
          {ready && session ? <>
            <Text>Continue as {session.user.githubLogin || session.user.name}. Signing out in either place signs out both.</Text>
            <Group><Button onClick={() => void connect()} loading={busy}>Connect terminal</Button></Group>
          </> : <Text>Use Sign in above to sign in with GitHub, then connect your terminal.</Text>}
          {error && <Text role="alert" c="red">{error}</Text>}
        </>}
      </Stack>
    </Alert>
  );
}
