import { loadMockDashboard } from './dashboardStream';
import type { DashboardSnapshot } from '../contracts/dashboard';

/**
 * Offline / test helper. Live loading uses useDashboardStream + WebSocket.
 */
export async function loadDashboard(): Promise<DashboardSnapshot> {
  return loadMockDashboard();
}
