import orgMock from '../mocks/orgPractice.mock.json';
import type { OrgPracticeView } from '../contracts/practice';
import { parseOrgPracticeView } from './parsePractice';

export async function loadOrgPractice(): Promise<OrgPracticeView> {
  return parseOrgPracticeView(orgMock);
}
