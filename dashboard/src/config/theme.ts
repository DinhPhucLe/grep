import { createTheme } from '@mantine/core';

const SPACE_MONO = '"Space Mono", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace';

export const dashboardTheme = createTheme({
  primaryColor: 'terminal',
  defaultRadius: 0,
  fontFamily: SPACE_MONO,
  fontFamilyMonospace: SPACE_MONO,
  headings: {
    fontFamily: SPACE_MONO,
    fontWeight: '700',
  },
  black: '#000000',
  white: '#FFFFFF',
  colors: {
    terminal: [
      '#FFF5F5',
      '#FFE3E3',
      '#FFC9C9',
      '#FFA8A8',
      '#FF8787',
      '#FF4E4E',
      '#E03131',
      '#C92A2A',
      '#A61E1E',
      '#8B1818',
    ],
    mint: [
      '#F0FBF7',
      '#D8F5EB',
      '#A3E6D2',
      '#7DD9C0',
      '#57CBAE',
      '#3BB89A',
      '#2E947C',
      '#247060',
      '#1B5448',
      '#143F36',
    ],
    cga: [
      '#F5F6FF',
      '#E8EBFF',
      '#D4D9FF',
      '#B8C0FF',
      '#9AA5FF',
      '#7B88F5',
      '#5F6BD4',
      '#4A54A8',
      '#3A4280',
      '#2C325F',
    ],
    retro: [
      '#FFF9EB',
      '#FFF0C9',
      '#FFE5A0',
      '#FFD166',
      '#F5C14A',
      '#E0A82E',
      '#B8871F',
      '#8F6917',
      '#6B4E11',
      '#4A360C',
    ],
  },
  primaryShade: 5,
  other: {
    accentCortisol: '#FF4E4E',
    accentMint: '#A3E6D2',
    accentCga: '#B8C0FF',
    accentYellow: '#FFD166',
    hardShadow: '4px 4px 0px 0px #000000',
    borderStrong: '2px solid #000000',
  },
  components: {
    Paper: {
      defaultProps: {
        radius: 0,
        withBorder: true,
      },
      styles: {
        root: {
          borderWidth: 2,
          borderColor: '#000000',
          boxShadow: '4px 4px 0px 0px #000000',
        },
      },
    },
    Button: {
      defaultProps: {
        radius: 0,
      },
      styles: {
        root: {
          borderWidth: 2,
          borderColor: '#000000',
          fontWeight: 700,
          textTransform: 'uppercase' as const,
          letterSpacing: '1px',
        },
      },
    },
    Badge: {
      defaultProps: {
        radius: 0,
      },
      styles: {
        root: {
          borderWidth: 2,
          borderColor: '#000000',
          textTransform: 'uppercase' as const,
          letterSpacing: '1px',
          fontWeight: 700,
        },
      },
    },
  },
});
