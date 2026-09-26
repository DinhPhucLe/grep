import { createTheme } from '@mantine/core';

export const dashboardTheme = createTheme({
  primaryColor: 'cyan',
  defaultRadius: 'md',
  fontFamily: 'Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
  fontFamilyMonospace: '"SFMono-Regular", Consolas, "Liberation Mono", monospace',
  headings: {
    fontFamily: 'Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
    fontWeight: '650',
  },
  colors: {
    observatory: [
      '#eefcff',
      '#d6f7fb',
      '#a8edf4',
      '#71e1ec',
      '#43d2df',
      '#27b8c6',
      '#168f9d',
      '#16727e',
      '#185c65',
      '#164d55'
    ],
  },
});
