import { parseOrgKnowledgeSummary } from './parseKnowledge';
import type { OrgKnowledgeSummary } from '../contracts/knowledge';

const apiBase = () => {
  const fromEnv = import.meta.env.VITE_API_BASE_URL;
  if (typeof fromEnv === 'string' && fromEnv.trim() !== '') {
    return fromEnv.replace(/\/+$/, '');
  }
  return '';
};

export async function loadOrgKnowledge(organizationId: string): Promise<OrgKnowledgeSummary> {
  const url = `${apiBase()}/api/v1/dashboard/organizations/${encodeURIComponent(organizationId)}/knowledge`;
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`Knowledge summary failed (${response.status})`);
  }
  return parseOrgKnowledgeSummary(await response.json());
}
