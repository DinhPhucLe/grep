import { Group, Text } from '@mantine/core';

const items = [
  { className: 'legend-swatch--retained', label: 'Retained change' },
  { className: 'legend-swatch--deleted', label: 'Deleted / replaced' },
  { className: 'legend-swatch--churn', label: 'Repeated churn' },
];

export function HeatmapLegend() {
  return (
    <Group gap="md" className="heatmap-legend">
      {items.map((item) => (
        <Group key={item.label} gap={6} wrap="nowrap">
          <span className={`legend-swatch ${item.className}`} aria-hidden="true" />
          <Text className="secondary-text" fz="xs" tt="uppercase">
            {item.label}
          </Text>
        </Group>
      ))}
    </Group>
  );
}
