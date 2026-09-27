import { PieChart } from '@mantine/charts';
import { Paper, Text, Title } from '@mantine/core';
import type { OutcomePie } from '../../contracts/practice';

export function OutcomePieCard({ pie }: { pie: OutcomePie }) {
  return (
    <Paper className="metric-card" p="md" radius={0} withBorder>
      <Text className="eyebrow">Outcomes</Text>
      <Title order={3} fz="lg" mt={4} mb="xs" className="section-title">
        Quiz results
      </Title>
      <Text className="secondary-text" mb="md">
        {pie.display.primary}
      </Text>
      {pie.status === 'available' ? (
        <PieChart
          h={220}
          data={pie.segments.map((segment) => ({
            name: segment.label,
            value: segment.value,
            color: segment.label === 'correct' ? 'teal.6' : 'red.6',
          }))}
          withLabelsLine
          labelsPosition="outside"
          labelsType="percent"
          withTooltip
        />
      ) : (
        <Text>No outcome data</Text>
      )}
    </Paper>
  );
}
