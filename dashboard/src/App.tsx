import type { DashboardSnapshot } from './contracts/dashboard';
import type { StreamStatus } from './data/dashboardStream';
import { DashboardPage } from './components/DashboardPage';

export function App({
  snapshot,
  status,
}: {
  snapshot: DashboardSnapshot;
  status: StreamStatus;
}) {
  return <DashboardPage snapshot={snapshot} status={status} />;
}
