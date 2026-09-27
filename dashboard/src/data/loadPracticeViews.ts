import type { EmployeePracticeView, OrgPracticeView } from '../contracts/practice';
import { parseEmployeePracticeView, parseOrgPracticeView } from './parsePractice';

const apiBase = () => (import.meta.env.VITE_API_BASE as string | undefined) ?? '';

export async function loadEmployeePractice(userId: string, practice: string): Promise<EmployeePracticeView> {
  const year = new Date().getUTCFullYear();
  const response = await fetch(
    `${apiBase()}/api/v1/dashboard/people/${encodeURIComponent(userId)}/practices/${encodeURIComponent(practice)}?year=${year}`,
  );
  if (!response.ok) {
    throw new Error(`Employee practice request failed (${response.status})`);
  }
  return parseEmployeePracticeView(await response.json());
}

export async function loadOrgPractice(orgId: string, practice: string): Promise<OrgPracticeView> {
  const response = await fetch(
    `${apiBase()}/api/v1/dashboard/organizations/${encodeURIComponent(orgId)}/practices/${encodeURIComponent(practice)}`,
  );
  if (!response.ok) {
    throw new Error(`Organization practice request failed (${response.status})`);
  }
  return parseOrgPracticeView(await response.json());
}
