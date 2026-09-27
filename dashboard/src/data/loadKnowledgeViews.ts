import type { EmployeeKnowledgeView, OrgKnowledgeView } from '../contracts/knowledge';
import { parseEmployeeKnowledgeView, parseOrgKnowledgeView } from './parseKnowledge';

const apiBase = () => (import.meta.env.VITE_API_BASE as string | undefined) ?? '';

export async function loadEmployeeKnowledge(userId: string): Promise<EmployeeKnowledgeView> {
  const year = new Date().getUTCFullYear();
  const response = await fetch(
    `${apiBase()}/api/v1/dashboard/people/${encodeURIComponent(userId)}/knowledge?year=${year}`,
  );
  if (!response.ok) {
    throw new Error(`Employee knowledge request failed (${response.status})`);
  }
  return parseEmployeeKnowledgeView(await response.json());
}

export async function loadOrgKnowledge(orgId: string): Promise<OrgKnowledgeView> {
  const response = await fetch(
    `${apiBase()}/api/v1/dashboard/organizations/${encodeURIComponent(orgId)}/knowledge`,
  );
  if (!response.ok) {
    throw new Error(`Organization knowledge request failed (${response.status})`);
  }
  return parseOrgKnowledgeView(await response.json());
}
