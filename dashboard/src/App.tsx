import type { EmployeePracticeView, OrgPracticeView } from './contracts/practice';
import { EmployeePracticePage } from './components/practice/EmployeePracticePage';
import { OrgPracticePage } from './components/practice/OrgPracticePage';
import { LandingPage } from './components/LandingPage';

export type AppRoute =
  | { kind: 'landing' }
  | { kind: 'people'; view: EmployeePracticeView }
  | { kind: 'organizations'; view: OrgPracticeView };

export function App({ route }: { route: AppRoute }) {
  switch (route.kind) {
    case 'people':
      return <EmployeePracticePage view={route.view} />;
    case 'organizations':
      return <OrgPracticePage view={route.view} />;
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
