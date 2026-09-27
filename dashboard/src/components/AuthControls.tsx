import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Anchor,
  Avatar,
  Button,
  Code,
  Group,
  Menu,
  Modal,
  Stack,
  Text,
  UnstyledButton,
} from '@mantine/core';
import { useAuth } from '../auth/AuthProvider';
import { pollGitHubDevice, startGitHubDevice, storeSession } from '../auth/session';

export function AuthControls() {
  const { session, ready, setSession, logout } = useAuth();
  const [modalOpen, setModalOpen] = useState(false);
  const [userCode, setUserCode] = useState('');
  const [verificationUri, setVerificationUri] = useState('https://github.com/login/device');
  const [status, setStatus] = useState('');
  const [busy, setBusy] = useState(false);
  const [logoutError, setLogoutError] = useState('');
  const deviceCodeRef = useRef('');
  const intervalRef = useRef(5);
  const pollTimer = useRef<number | null>(null);

  const stopPolling = useCallback(() => {
    if (pollTimer.current != null) {
      window.clearTimeout(pollTimer.current);
      pollTimer.current = null;
    }
  }, []);

  const schedulePoll = useCallback(() => {
    stopPolling();
    const delayMs = Math.max(1, intervalRef.current) * 1000;
    pollTimer.current = window.setTimeout(async () => {
      let result;
      try {
        result = await pollGitHubDevice(deviceCodeRef.current);
      } catch (error) {
        setBusy(false);
        setStatus(error instanceof Error ? error.message : 'Could not check GitHub authorization. Please try again.');
        return;
      }
      if (result.status === 'pending') {
        if (result.code === 'slow_down') {
          intervalRef.current += 1;
        }
        setStatus('Waiting for GitHub authorization…');
        schedulePoll();
        return;
      }
      if (result.status === 'error') {
        setBusy(false);
        setStatus(result.message);
        return;
      }
      storeSession(result.session);
      setSession(result.session);
      setBusy(false);
      setStatus('Signed in');
      setModalOpen(false);
      stopPolling();
    }, delayMs);
  }, [setSession, stopPolling]);

  useEffect(() => () => stopPolling(), [stopPolling]);

  const beginSignIn = async () => {
    stopPolling();
    setBusy(true);
    setStatus('Starting GitHub device login…');
    setUserCode('');
    setModalOpen(true);
    try {
      const start = await startGitHubDevice();
      deviceCodeRef.current = start.deviceCode;
      intervalRef.current = start.interval > 0 ? start.interval : 5;
      setUserCode(start.userCode);
      setVerificationUri(start.verificationUri);
      setStatus('Authorize in GitHub, then this page will finish signing you in.');
      schedulePoll();
    } catch (error) {
      setBusy(false);
      setStatus(error instanceof Error ? error.message : 'Sign in failed');
    }
  };

  const closeModal = () => {
    if (busy && !session) {
      stopPolling();
      setBusy(false);
    }
    setModalOpen(false);
  };

  if (!ready) {
    return null;
  }

  return (
    <>
      <div className="auth-controls">
        {session ? (
          <Menu shadow="md" width={220} position="bottom-end" withinPortal>
            <Menu.Target>
              <UnstyledButton
                className="auth-avatar-button"
                aria-label={`Account menu for ${session.user.githubLogin || session.user.name}`}
              >
                <Avatar
                  src={session.user.avatarUrl || undefined}
                  alt={session.user.name}
                  radius={0}
                  size={40}
                  color="dark"
                >
                  {(session.user.githubLogin || session.user.name || '?').slice(0, 2).toUpperCase()}
                </Avatar>
              </UnstyledButton>
            </Menu.Target>
            <Menu.Dropdown className="auth-menu">
              <Menu.Label>
                {session.user.githubLogin || session.user.name}
                {session.organization.name ? ` · ${session.organization.name}` : ''}
              </Menu.Label>
              <Menu.Item
                color="red"
                onClick={() => {
                  setLogoutError('');
                  void logout().catch(() => setLogoutError('Could not sign out. Please try again.'));
                }}
              >
                Log out
              </Menu.Item>
            </Menu.Dropdown>
          </Menu>
        ) : (
          <Button className="auth-signin" radius={0} onClick={() => void beginSignIn()}>
            Sign in
          </Button>
        )}
      </div>
      {logoutError && <Text role="alert" c="red">{logoutError}</Text>}

      <Modal
        opened={modalOpen}
        onClose={closeModal}
        title="Sign in with GitHub"
        centered
        radius={0}
        classNames={{ content: 'auth-modal', header: 'auth-modal' }}
      >
        <Stack gap="md">
          {userCode ? (
            <>
              <Text className="secondary-text">
                Open GitHub, enter the code below, and approve sign-in. This page will sign you in automatically.
              </Text>
              <Group gap="sm" align="center">
                <Text fw={700}>Code</Text>
                <Code className="auth-user-code">{userCode}</Code>
              </Group>
              <Anchor href={verificationUri} target="_blank" rel="noreferrer">
                {verificationUri}
              </Anchor>
            </>
          ) : null}
          {status ? <Text size="sm">{status}</Text> : null}
          <Group justify="flex-end">
            <Button variant="default" radius={0} onClick={closeModal}>
              Cancel
            </Button>
          </Group>
        </Stack>
      </Modal>
    </>
  );
}
