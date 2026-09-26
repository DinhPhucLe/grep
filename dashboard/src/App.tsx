import type { DashboardSnapshot } from './contracts/dashboard';
import { DashboardPage } from './components/DashboardPage';

export function App({ snapshot }: { snapshot: DashboardSnapshot }) {
  return <DashboardPage snapshot={snapshot} />;
}
