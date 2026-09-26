import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { Alert, MantineProvider, Stack, Text, Title } from '@mantine/core';
import '@mantine/core/styles.css';
import '@mantine/charts/styles.css';
import './styles/dashboard.css';
import { App } from './App';
import { dashboardTheme } from './config/theme';
import { useDashboardStream } from './data/useDashboardStream';

function Root() {
  const { snapshot, status, error } = useDashboardStream();

  if (error && !snapshot) {
    return (
      <Stack maw={720} mx="auto" p="xl" mt="xl">
        <Title order={1}>Dashboard data could not be loaded</Title>
        <Alert color="red" title="Metrics stream error">
          <Text>{error}</Text>
        </Alert>
      </Stack>
    );
  }

  if (!snapshot) {
    return (
      <Stack maw={720} mx="auto" p="xl" mt="xl">
        <Title order={1}>Waiting for session</Title>
        <Text c="dimmed">Connecting to cortisol-server ({status})…</Text>
        {error ? (
          <Alert color="yellow" title="Stream warning">
            <Text>{error}</Text>
          </Alert>
        ) : null}
      </Stack>
    );
  }

  return <App snapshot={snapshot} status={status} />;
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <MantineProvider theme={dashboardTheme} forceColorScheme="light">
      <Root />
    </MantineProvider>
  </StrictMode>,
);
