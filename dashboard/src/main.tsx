import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { Alert, MantineProvider, Stack, Text, Title } from '@mantine/core';
import '@mantine/core/styles.css';
import '@mantine/charts/styles.css';
import './styles/dashboard.css';
import { App } from './App';
import { dashboardTheme } from './config/theme';
import { loadDashboard } from './data/loadDashboard';

const root = createRoot(document.getElementById('root')!);

function render(content: React.ReactNode) {
  root.render(
    <StrictMode>
      <MantineProvider theme={dashboardTheme} defaultColorScheme="auto">
        {content}
      </MantineProvider>
    </StrictMode>,
  );
}

loadDashboard()
  .then((snapshot) => render(<App snapshot={snapshot} />))
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
