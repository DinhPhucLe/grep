import type {
  OrgRecommendation,
  PersonRecommendation,
  RecommendationsResponse,
} from '../contracts/search';

const apiBase = () => (import.meta.env.VITE_API_BASE as string | undefined) ?? '';

async function getJSON<T>(path: string): Promise<T> {
  const response = await fetch(`${apiBase()}${path}`);
  if (!response.ok) {
    throw new Error(`Request failed (${response.status}) for ${path}`);
  }
  return (await response.json()) as T;
}

export async function loadPeopleRecommendations(query = ''): Promise<PersonRecommendation[]> {
  const params = new URLSearchParams();
  if (query.trim()) {
    params.set('q', query.trim());
  }
  const suffix = params.size > 0 ? `?${params}` : '';
  const body = await getJSON<RecommendationsResponse<PersonRecommendation>>(
    `/api/v1/dashboard/people/recommendations${suffix}`,
  );
  return body.items ?? [];
}

export async function loadOrgRecommendations(query = ''): Promise<OrgRecommendation[]> {
  const params = new URLSearchParams();
  if (query.trim()) {
    params.set('q', query.trim());
  }
  const suffix = params.size > 0 ? `?${params}` : '';
  const body = await getJSON<RecommendationsResponse<OrgRecommendation>>(
    `/api/v1/dashboard/organizations/recommendations${suffix}`,
  );
  return body.items ?? [];
}
