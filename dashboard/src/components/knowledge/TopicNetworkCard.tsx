import { useEffect, useMemo, useRef, useState } from 'react';
import { Group } from '@visx/group';
import { ParentSize } from '@visx/responsive';
import { Paper, Text, Title } from '@mantine/core';
import {
  forceCenter,
  forceLink,
  forceManyBody,
  forceSimulation,
  type SimulationNodeDatum,
} from 'd3-force';
import type { TopicNetwork } from '../../contracts/knowledge';
import { practiceIntensityColor } from '../../utils/practiceIntensity';

interface SimNode extends SimulationNodeDatum {
  id: string;
  label: string;
  weight: number;
}

interface SimLink {
  source: string | SimNode;
  target: string | SimNode;
  weight: number;
}

function weightIntensity(weight: number, maxWeight: number): number {
  if (weight <= 0 || maxWeight <= 0) return 0;
  if (maxWeight === 1) return 1;
  return Math.min(4, Math.max(1, 1 + Math.floor((weight * 4) / maxWeight)));
}

function TopicNetworkChart({
  network,
  width,
  height,
}: {
  network: TopicNetwork;
  width: number;
  height: number;
}) {
  const maxWeight = Math.max(1, ...network.nodes.map((n) => n.weight));
  const [nodes, setNodes] = useState<SimNode[]>([]);
  const [links, setLinks] = useState<SimLink[]>([]);
  const frame = useRef(0);

  const seed = useMemo(
    () => ({
      nodes: network.nodes.map((n) => ({ ...n })),
      links: network.links.map((l) => ({ ...l })),
    }),
    [network],
  );

  useEffect(() => {
    const simNodes: SimNode[] = seed.nodes.map((n) => ({ ...n }));
    const simLinks: SimLink[] = seed.links.map((l) => ({ ...l }));
    const simulation = forceSimulation(simNodes)
      .force(
        'link',
        forceLink<SimNode, SimLink>(simLinks)
          .id((d) => d.id)
          .distance(64)
          .strength(0.4),
      )
      .force('charge', forceManyBody().strength(-180))
      .force('center', forceCenter(width / 2, height / 2))
      .on('tick', () => {
        cancelAnimationFrame(frame.current);
        frame.current = requestAnimationFrame(() => {
          setNodes(simNodes.map((n) => ({ ...n })));
          setLinks(simLinks.map((l) => ({ ...l })));
        });
      });
    return () => {
      simulation.stop();
      cancelAnimationFrame(frame.current);
    };
  }, [seed, width, height]);

  if (network.status !== 'available' || network.nodes.length === 0) {
    return (
      <Text className="secondary-text" p="md">
        No topic network data
      </Text>
    );
  }

  return (
    <svg width={width} height={height} role="img" aria-label="Topic co-occurrence network">
      <Group>
        {links.map((link, i) => {
          const source = link.source as SimNode;
          const target = link.target as SimNode;
          if (source.x == null || target.x == null) return null;
          return (
            <line
              key={`${source.id}-${target.id}-${i}`}
              x1={source.x}
              y1={source.y}
              x2={target.x}
              y2={target.y}
              stroke="#c8c8c8"
              strokeWidth={Math.max(1, Math.min(4, link.weight))}
            />
          );
        })}
        {nodes.map((node) => {
          if (node.x == null || node.y == null) return null;
          const r = 8 + Math.min(14, node.weight);
          const fill = practiceIntensityColor(weightIntensity(node.weight, maxWeight));
          return (
            <g key={node.id} transform={`translate(${node.x},${node.y})`}>
              <circle r={r} fill={fill} stroke="#333" strokeWidth={1}>
                <title>{`${node.label}: ${node.weight}`}</title>
              </circle>
              <text
                y={r + 12}
                textAnchor="middle"
                fontSize={11}
                fill="#222"
                style={{ pointerEvents: 'none' }}
              >
                {node.label}
              </text>
            </g>
          );
        })}
      </Group>
    </svg>
  );
}

export function TopicNetworkCard({ network }: { network: TopicNetwork }) {
  return (
    <Paper className="heatmap-panel" p="md" radius={0} withBorder>
      <Text className="eyebrow">Topics</Text>
      <Title order={3} fz="lg" mt={4} mb="md" className="section-title">
        Topic network
      </Title>
      <div style={{ width: '100%', height: 360 }}>
        <ParentSize>
          {({ width, height }) =>
            width > 0 && height > 0 ? (
              <TopicNetworkChart network={network} width={width} height={height} />
            ) : null
          }
        </ParentSize>
      </div>
    </Paper>
  );
}
