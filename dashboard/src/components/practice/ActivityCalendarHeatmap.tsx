import { Paper, Text, Title } from '@mantine/core';
import type { ActivityCalendar } from '../../contracts/practice';
import { practiceIntensityColor } from '../../utils/practiceIntensity';

export function ActivityCalendarHeatmap({
  calendar,
  year,
  title,
  ariaLabel,
}: {
  calendar: ActivityCalendar;
  year: number;
  title?: string;
  ariaLabel?: string;
}) {
  const byDate = new Map(calendar.days.map((day) => [day.date, day]));
  const start = new Date(Date.UTC(year, 0, 1));
  const end = new Date(Date.UTC(year, 11, 31));
  const cells: { date: string; intensity: number; count: number }[] = [];
  for (let d = new Date(start); d <= end; d.setUTCDate(d.getUTCDate() + 1)) {
    const date = d.toISOString().slice(0, 10);
    const hit = byDate.get(date);
    cells.push({ date, intensity: hit?.intensity ?? 0, count: hit?.count ?? 0 });
  }
  const heading = title ?? `Practice calendar ${year}`;
  const label = ariaLabel ?? `Practice activity calendar for ${year}`;

  return (
    <Paper className="heatmap-panel" p="md" radius={0} withBorder>
      <Text className="eyebrow">Activity</Text>
      <Title order={3} fz="lg" mt={4} mb="md" className="section-title">
        {heading}
      </Title>
      <div className="practice-calendar" role="img" aria-label={label}>
        {cells.map((cell) => (
          <div
            key={cell.date}
            className="practice-calendar-cell"
            title={`${cell.date}: ${cell.count}`}
            data-intensity={cell.intensity}
            style={{ background: practiceIntensityColor(cell.intensity) }}
          />
        ))}
      </div>
    </Paper>
  );
}
