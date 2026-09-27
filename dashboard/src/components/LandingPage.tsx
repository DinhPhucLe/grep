import { useCallback } from 'react';
import { Container, Group, Paper, Stack, Text, Title } from '@mantine/core';
import { dashboardLayout } from '../config/layout';
import {
  loadOrgRecommendations,
  loadPeopleRecommendations,
} from '../data/loadRecommendations';
import { DirectorySearch } from './DirectorySearch';

export function LandingPage() {
  const loadPeople = useCallback(async (query: string) => {
    const items = await loadPeopleRecommendations(query);
    return items.map((item) => ({
      id: item.id,
      primary: item.name,
      secondary: item.mail,
    }));
  }, []);

  const loadOrgs = useCallback(async (query: string) => {
    const items = await loadOrgRecommendations(query);
    return items.map((item) => ({
      id: item.id,
      primary: item.name,
    }));
  }, []);

  return (
    <Container
      component="main"
      size={dashboardLayout.maxWidth}
      px={dashboardLayout.pagePadding}
      py={{ base: 'md', sm: 'xl' }}
    >
      <Stack gap={dashboardLayout.sectionGap}>
        <Paper className="dashboard-header" p={{ base: 'lg', sm: 'xl' }} radius={0} withBorder>
          <Stack gap="lg">
            <Group gap="sm">
              <div className="brand-mark" aria-hidden="true">
                VO
              </div>
              <div>
                <Text className="eyebrow">Vibe Coding Observatory</Text>
                <Title order={1} className="section-title">
                  Dashboard
                </Title>
              </div>
            </Group>

            <Group align="flex-start" gap="md" wrap="wrap">
              <DirectorySearch
                label="Search people"
                placeholder="Name or mail"
                loadSuggestions={loadPeople}
                onSelect={(id) => {
                  window.location.assign(`/people/${encodeURIComponent(id)}`);
                }}
              />
              <DirectorySearch
                label="Search organizations"
                placeholder="Organization name"
                loadSuggestions={loadOrgs}
                onSelect={(id) => {
                  window.location.assign(`/organizations/${encodeURIComponent(id)}`);
                }}
              />
            </Group>
          </Stack>
        </Paper>

        <Paper className="summary-panel" p="xl" radius={0} withBorder>
          <Text className="eyebrow">Empty workspace</Text>
          <Title order={2} className="section-title" mt={4}>
            Select a person or organization
          </Title>
          <Text className="secondary-text" mt="sm">
            Open a search above to load recommendations from the database. Choosing a result opens
            that entry&apos;s practice dashboard.
          </Text>
        </Paper>
      </Stack>
    </Container>
  );
}
