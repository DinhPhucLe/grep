import { Paper, Text, Title } from '@mantine/core';
import type { CodebaseTreemap, TreemapNode } from '../../contracts/practice';

const COLORS = ['#d0d0d0', '#ffc9c9', '#ff8f8f', '#ff4e4e', '#b01010'];

function NodeBox({ node, depth }: { node: TreemapNode; depth: number }) {
  const children = node.children ?? [];
  return (
    <div
      className="treemap-node"
      style={{
        background: COLORS[node.intensity] ?? COLORS[0],
        flex: Math.max(node.value, 1),
        minHeight: depth === 0 ? 160 : 48,
      }}
      title={`${node.name}: ${node.value}`}
    >
      <Text fz="xs" fw={700} tt="uppercase" c="#000">
        {node.name} ({node.value})
      </Text>
      {children.length > 0 ? (
        <div className="treemap-row">
          {children.map((child) => (
            <NodeBox key={child.name} node={child} depth={depth + 1} />
          ))}
        </div>
      ) : null}
    </div>
  );
}

export function CodebaseTreemapCard({ treemap }: { treemap: CodebaseTreemap }) {
  return (
    <Paper className="heatmap-panel" p="md" radius={0} withBorder>
      <Text className="eyebrow">Codebase</Text>
      <Title order={3} fz="lg" mt={4} mb="md" className="section-title">
        {treemap.repoName}
      </Title>
      <NodeBox node={treemap.root} depth={0} />
    </Paper>
  );
}
