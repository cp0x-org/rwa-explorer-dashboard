import { useMemo, ReactNode } from 'react';

// material-ui
import { createTheme, StyledEngineProvider, ThemeOptions, ThemeProvider, Theme, TypographyVariantsOptions } from '@mui/material/styles';
import CssBaseline from '@mui/material/CssBaseline';

// project imports
import useConfig from 'hooks/useConfig';
import Palette from './palette';
import Typography from './typography';

import componentStyleOverrides from './compStyleOverride';
import customShadows from './shadows';

// types
import { CustomShadowProps } from 'types/default-theme';

interface Props {
  children: ReactNode;
}

export default function ThemeCustomization({ children }: Props) {
  const { borderRadius, fontFamily, mode, outlinedFilled, presetColor, themeDirection } = useConfig();

  const theme: Theme = useMemo<Theme>(() => Palette(mode, presetColor), [mode, presetColor]);

  const themeTypography: TypographyVariantsOptions = useMemo<TypographyVariantsOptions>(
    () => Typography(theme, borderRadius, fontFamily),
    [theme, borderRadius, fontFamily]
  );
  const themeCustomShadows: CustomShadowProps = useMemo<CustomShadowProps>(() => customShadows(mode, theme), [mode, theme]);

  const themeOptions: ThemeOptions = useMemo(
    () => ({
      direction: themeDirection,
      palette: theme.palette,
      mixins: {
        toolbar: {
          minHeight: '48px',
          padding: '16px',
          '@media (min-width: 600px)': {
            minHeight: '48px'
          }
        }
      },
      typography: themeTypography,
      customShadows: themeCustomShadows
    }),
    [themeDirection, theme, themeCustomShadows, themeTypography]
  );

  const themes: Theme = createTheme(themeOptions);
  themes.components = useMemo(() => componentStyleOverrides(themes, borderRadius, outlinedFilled), [themes, borderRadius, outlinedFilled]);

  const content = (
    <ThemeProvider theme={themes}>
      <CssBaseline enableColorScheme />
      {children}
    </ThemeProvider>
  );

  // MUI's injectFirst cache is one per process and browser-only; the prerender gives every page a fresh cache instead
  // (entry-server.tsx) and puts the collected styles first in <head>, as injectFirst does in the browser.
  if (typeof document === 'undefined') return content;
  return <StyledEngineProvider injectFirst>{content}</StyledEngineProvider>;
}
