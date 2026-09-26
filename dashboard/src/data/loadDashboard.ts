import mockDashboard from '../mocks/dashboard.mock.json';
import type { DashboardSnapshot } from '../contracts/dashboard';
import { parseDashboardSnapshot } from './parseDashboard';

/**
 * This is the only current data-source decision. Replace the mock with a fetch
 * from the existing Go server when its dashboard endpoint is available.
 */
export async function loadDashboard(): Promise<DashboardSnapshot> {
  return parseDashboardSnapshot(mockDashboard);
}
