import { LineChart } from '@mantine/charts';
import { Paper, Text, Title } from '@mantine/core';
import type { OrgPracticeView } from '../../contracts/practice';

export function PracticeTimeseriesCard({
  timeseries,
}: {
  timeseries: OrgPracticeView['timeseries'];
}) {
  const data = timeseries.points.map((point) => ({
    date: point.t,
    correct: point.correct,
    failedReveal: point.failedReveal,
    medianActiveAnswerTimeMs: point.medianActiveAnswerTimeMs ?? 0,
  }));

  return (
    <Paper className="metric-card" p="md" radius={0} withBorder>
      <Text className="eyebrow">Trend</Text>
      <Title order={3} fz="lg" mt={4} mb="md" className="section-title">
        Outcomes over time
      </Title>
      {timeseries.status === 'available' && data.length > 0 ? (
        <LineChart
          h={280}
          data={data}
          dataKey="date"
          series={[
            { name: 'correct', color: 'teal.6' },
            { name: 'failedReveal', color: 'red.6' },
          ]}
          curveType="step"
          withLegend
          gridAxis="xy"
        />
      ) : (
        <Text>No timeseries data</Text>
      )}
    </Paper>
  );
}
