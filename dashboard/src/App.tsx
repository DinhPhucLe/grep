import type { EmployeeKnowledgeView } from './contracts/knowledge';
import type { EmployeePracticeView, OrgPracticeView } from './contracts/practice';
import type { OrgKnowledgeView } from './contracts/knowledge';
import { EmployeePracticePage } from './components/practice/EmployeePracticePage';
import { OrgPracticePage } from './components/practice/OrgPracticePage';
import { LandingPage } from './components/LandingPage';

export type AppRoute =
  | { kind: 'landing' }
  | { kind: 'people'; view: EmployeePracticeView; knowledge?: EmployeeKnowledgeView | null }
  | { kind: 'organizations'; view: OrgPracticeView; knowledge?: OrgKnowledgeView | null };

export function App({ route }: { route: AppRoute }) {
  switch (route.kind) {
    case 'people':
      return <EmployeePracticePage view={route.view} knowledge={route.knowledge} />;
    case 'organizations':
      return <OrgPracticePage view={route.view} knowledge={route.knowledge} />;
    default:
      return <LandingPage />;
  }
}

export function resolveRoutePath(pathname: string): {
  kind: 'landing' | 'people' | 'organizations';
  id?: string;
} {
  const path = pathname.replace(/\/+$/, '') || '/';
  const people = path.match(/^\/people\/([^/]+)$/);
  if (people) {
    return { kind: 'people', id: people[1] };
  }
  const org = path.match(/^\/organizations\/([^/]+)$/);
  if (org) {
    return { kind: 'organizations', id: org[1] };
  }
  return { kind: 'landing' };
}
