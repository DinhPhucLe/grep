import { useEffect, useMemo, useState } from 'react';
import { Badge, Group, Paper, Stack, Text, Title } from '@mantine/core';
import type { CodeHeatmap as CodeHeatmapModel } from '../../contracts/dashboard';
import { FileSelector } from './FileSelector';
import { HeatmapLegend } from './HeatmapLegend';

export function CodeHeatmap({ heatmap }: { heatmap: CodeHeatmapModel }) {
  const [selectedPath, setSelectedPath] = useState(heatmap.files[0]?.path ?? '');

  useEffect(() => {
    if (!heatmap.files.some((file) => file.path === selectedPath)) {
      setSelectedPath(heatmap.files[0]?.path ?? '');
    }
  }, [heatmap.files, selectedPath]);

  const selectedFile = useMemo(
    () => heatmap.files.find((file) => file.path === selectedPath) ?? heatmap.files[0],
    [heatmap.files, selectedPath],
  );

  return (
    <Paper className="heatmap-panel" radius="xl" withBorder>
      <Stack gap={0}>
        <Group className="heatmap-panel__header" justify="space-between" align="flex-start">
          <div>
            <Text className="eyebrow">Code-change evidence</Text>
            <Title order={2} fz="xl" mt={4}>
              Session line heatmap
            </Title>
            <Text c="dimmed" fz="sm" mt={6}>
              Retained changes and deletion/replacement churn observed during this session.
            </Text>
          </div>
          <Badge color={heatmap.mode === 'history' ? 'cyan' : 'gray'} variant="light">
            {heatmap.mode === 'history' ? 'Session history' : 'Final diff only'}
          </Badge>
        </Group>

        <div className="heatmap-panel__toolbar">
          <FileSelector
            files={heatmap.files}
            selectedPath={selectedPath}
            onSelect={setSelectedPath}
          />
          <HeatmapLegend />
        </div>

        {heatmap.mode === 'final_diff' ? (
          <Text className="heatmap-notice" fz="xs">
            Intermediate edit history is unavailable. Churn intensity is not inferred from the final diff.
          </Text>
        ) : null}

        {selectedFile ? (
          <div className="code-frame" role="table" aria-label={`Changes in ${selectedFile.path}`}>
            <div className="code-frame__path">{selectedFile.path}</div>
            <div className="code-lines">
              {selectedFile.rows.map((row) => (
                <div
                  key={row.rowId}
                  className="code-row"
                  data-state={row.state}
                  data-intensity={row.intensity}
                  role="row"
                  title={row.churnCount > 0 ? `${row.churnCount} deletion/replacement event${row.churnCount === 1 ? '' : 's'}` : undefined}
                >
                  <span className="code-row__marker" aria-hidden="true">
                    {row.state === 'retained_change'
                      ? '+'
                      : row.state === 'deleted_or_replaced'
                        ? '−'
                        : ' '}
                  </span>
                  <span className="code-row__line" aria-label="Old line number">
                    {row.oldLineNumber ?? ''}
                  </span>
                  <span className="code-row__line" aria-label="New line number">
                    {row.newLineNumber ?? ''}
                  </span>
                  <code className="code-row__content">{row.content || ' '}</code>
                  {row.churnCount > 1 ? (
                    <span className="code-row__churn" aria-label={`${row.churnCount} churn events`}>
                      ×{row.churnCount}
                    </span>
                  ) : null}
                </div>
              ))}
            </div>
          </div>
        ) : (
          <Text p="xl" c="dimmed">
            No changed files were supplied.
          </Text>
        )}
      </Stack>
    </Paper>
  );
}
