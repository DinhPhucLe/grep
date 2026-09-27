/** Tier 0 = empty/parent shell; 1–4 = low → high relative activity. */
export const PRACTICE_INTENSITY_COLORS = [
  '#f0f0f0',
  '#A3E6D2',
  '#FFD166',
  '#ff8f8f',
  '#FF4E4E',
] as const;

export function practiceIntensityColor(intensity: number): string {
  const clamped = Math.max(0, Math.min(4, Math.floor(intensity)));
  return PRACTICE_INTENSITY_COLORS[clamped] ?? PRACTICE_INTENSITY_COLORS[0];
}
