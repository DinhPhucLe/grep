import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { Alert, MantineProvider, Stack, Text, Title } from '@mantine/core';
import '@mantine/core/styles.css';
import '@mantine/charts/styles.css';
import './styles/dashboard.css';
import { App, resolveRoutePath, type AppRoute } from './App';
import { dashboardTheme } from './config/theme';
import { loadEmployeePractice, loadOrgPractice } from './data/loadPracticeViews';

const root = createRoot(document.getElementById('root')!);

function render(content: React.ReactNode) {
  root.render(
    <StrictMode>
      <MantineProvider theme={dashboardTheme} forceColorScheme="light">
        {content}
      </MantineProvider>
    </StrictMode>,
  );
}

async function loadRoute(): Promise<AppRoute> {
  const resolved = resolveRoutePath(window.location.pathname);
  const params = new URLSearchParams(window.location.search);
  const practice = params.get('practice') ?? 'lead_and_reveal';
  if (resolved.kind === 'people') {
    if (!resolved.id) {
      throw new Error('Missing person id');
    }
    return { kind: 'people', view: await loadEmployeePractice(resolved.id, practice) };
  }
  if (resolved.kind === 'organizations') {
    if (!resolved.id) {
      throw new Error('Missing organization id');
    }
    return { kind: 'organizations', view: await loadOrgPractice(resolved.id, practice) };
  }
  return { kind: 'landing' };
}

loadRoute()
  .then((route) => render(<App route={route} />))
  .catch((error: unknown) => {
    const message = error instanceof Error ? error.message : 'Unknown dashboard data error';
    render(
      <Stack maw={720} mx="auto" p="xl" mt="xl">
        <Title order={1}>Dashboard data could not be loaded</Title>
        <Alert color="red" title="Invalid dashboard snapshot">
          <Text>{message}</Text>
        </Alert>
      </Stack>,
    );
  });
