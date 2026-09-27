import { AxisBottom, AxisLeft } from '@visx/axis';
import { curveStepAfter } from '@visx/curve';
import { GridRows } from '@visx/grid';
import { Group } from '@visx/group';
import { ParentSize } from '@visx/responsive';
import { scaleLinear, scaleTime } from '@visx/scale';
import { AreaClosed, LinePath } from '@visx/shape';
import { Paper, Text, Title } from '@mantine/core';
import type { OrgPracticeView } from '../../contracts/practice';
import { formatReadableDate } from '../../utils/formatPractice';

const CORRECT_FILL = '#A3E6D2';
const CORRECT_STROKE = '#1a8f6e';
const FAILED_FILL = '#FF4E4E';
const FAILED_STROKE = '#b01010';
const AXIS_COLOR = '#000000';

type ChartPoint = {
  date: Date;
  correct: number;
  failedReveal: number;
};

function parsePointDate(raw: string): Date {
  const day = raw.includes('T') ? raw.slice(0, 10) : raw.slice(0, 10);
  return new Date(`${day}T00:00:00.000Z`);
}

function OutcomesAreaChart({
  width,
  height,
  data,
}: {
  width: number;
  height: number;
  data: ChartPoint[];
}) {
  const margin = { top: 16, right: 16, bottom: 40, left: 40 };
  const innerWidth = Math.max(0, width - margin.left - margin.right);
  const innerHeight = Math.max(0, height - margin.top - margin.bottom);
  if (innerWidth < 16 || innerHeight < 16 || data.length === 0) {
    return null;
  }

  const dates = data.map((d) => d.date);
  const maxY = Math.max(
    1,
    ...data.map((d) => Math.max(d.correct, d.failedReveal)),
  );

  const xScale = scaleTime({
    domain: [dates[0], dates[dates.length - 1]],
    range: [0, innerWidth],
  });
  const yScale = scaleLinear({
    domain: [0, maxY],
    range: [innerHeight, 0],
    nice: true,
  });

  const x = (d: ChartPoint) => xScale(d.date) ?? 0;
  const yCorrect = (d: ChartPoint) => yScale(d.correct) ?? 0;
  const yFailed = (d: ChartPoint) => yScale(d.failedReveal) ?? 0;

  return (
    <svg width={width} height={height} style={{ imageRendering: 'pixelated' }}>
      <Group left={margin.left} top={margin.top}>
        <GridRows
          scale={yScale}
          width={innerWidth}
          stroke={AXIS_COLOR}
          strokeDasharray="3 3"
          numTicks={5}
        />
        <AreaClosed<ChartPoint>
          data={data}
          x={x}
          y={yCorrect}
          yScale={yScale}
          curve={curveStepAfter}
          fill={CORRECT_FILL}
          fillOpacity={0.85}
        />
        <AreaClosed<ChartPoint>
          data={data}
          x={x}
          y={yFailed}
          yScale={yScale}
          curve={curveStepAfter}
          fill={FAILED_FILL}
          fillOpacity={0.55}
        />
        <LinePath<ChartPoint>
          data={data}
          x={x}
          y={yCorrect}
          curve={curveStepAfter}
          stroke={CORRECT_STROKE}
          strokeWidth={2}
        />
        <LinePath<ChartPoint>
          data={data}
          x={x}
          y={yFailed}
          curve={curveStepAfter}
          stroke={FAILED_STROKE}
          strokeWidth={2}
        />
        <AxisLeft
          scale={yScale}
          stroke={AXIS_COLOR}
          tickStroke={AXIS_COLOR}
          numTicks={5}
          tickLabelProps={() => ({
            fill: AXIS_COLOR,
            fontSize: 11,
            fontFamily: 'monospace',
            textAnchor: 'end',
            dx: -4,
            dy: 3,
          })}
        />
        <AxisBottom
          top={innerHeight}
          scale={xScale}
          stroke={AXIS_COLOR}
          tickStroke={AXIS_COLOR}
          numTicks={Math.min(8, data.length)}
          tickFormat={(value) => {
            const date = value instanceof Date ? value : new Date(Number(value));
            return formatReadableDate(date.toISOString().slice(0, 10));
          }}
          tickLabelProps={() => ({
            fill: AXIS_COLOR,
            fontSize: 10,
            fontFamily: 'monospace',
            textAnchor: 'middle',
            dy: 4,
          })}
        />
      </Group>
    </svg>
  );
}

export function PracticeTimeseriesCard({
  timeseries,
}: {
  timeseries: OrgPracticeView['timeseries'];
}) {
  const data: ChartPoint[] = timeseries.points.map((point) => ({
    date: parsePointDate(point.t),
    correct: point.correct,
    failedReveal: point.failedReveal,
  }));

  return (
    <Paper className="metric-card" p="md" radius={0} withBorder>
      <Text className="eyebrow">Trend</Text>
      <Title order={3} fz="lg" mt={4} mb="md" className="section-title">
        Outcomes over time
      </Title>
      {timeseries.status === 'available' && data.length > 0 ? (
        <>
          <div className="practice-area-legend" aria-hidden>
            <span className="practice-area-legend-item">
              <span className="practice-area-swatch" style={{ background: CORRECT_FILL }} />
              correct
            </span>
            <span className="practice-area-legend-item">
              <span className="practice-area-swatch" style={{ background: FAILED_FILL }} />
              failedReveal
            </span>
          </div>
          <div style={{ width: '100%', height: 280 }}>
            <ParentSize>
              {({ width, height }) => (
                <OutcomesAreaChart width={width} height={height} data={data} />
              )}
            </ParentSize>
          </div>
        </>
      ) : (
        <Text>No timeseries data</Text>
      )}
    </Paper>
  );
}
