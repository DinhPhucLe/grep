import employeeMock from '../mocks/employeePractice.mock.json';
import type { EmployeePracticeView } from '../contracts/practice';
import { parseEmployeePracticeView } from './parsePractice';

export async function loadEmployeePractice(): Promise<EmployeePracticeView> {
  return parseEmployeePracticeView(employeeMock);
}
