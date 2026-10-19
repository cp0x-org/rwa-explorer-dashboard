// material-ui
import { Box, Button, Stack, Theme, useTheme } from '@mui/material';

// project imports
import { LocaleLink, useT } from 'i18n';

// Menu button styling as an object for reuse
const menuButtonStyle = (theme: Theme) => ({
  color: theme.palette.text.primary,
  fontWeight: 500,
  fontSize: '15px',
  textTransform: 'none',
  padding: '6px 16px',
  borderRadius: '8px',
  '&:hover': {
    backgroundColor: theme.palette.primary.light,
    color: theme.palette.primary.main
  }
});

export default function MenuItems() {
  const theme = useTheme();
  const t = useT();

  // below md the MobileMenu button replaces these links; CSS decides, so the prerendered header fits any width
  return (
    <Box component="nav" aria-label={t('header.siteMenu')} sx={{ display: { xs: 'none', md: 'flex' }, alignItems: 'center', ml: 3 }}>
      <Stack direction="row" spacing={1}>
        <Button component={LocaleLink} to="/" sx={menuButtonStyle(theme)}>
          {t('header.home')}
        </Button>
        <Button href="https://pi.cp0x.com" rel="noopener noreferrer" sx={menuButtonStyle(theme)}>
          {t('header.pi')}
        </Button>
        <Button href="https://cp0x.com" target="_blank" rel="noopener noreferrer" sx={menuButtonStyle(theme)}>
          {t('header.referrals')}
        </Button>
      </Stack>
    </Box>
  );
}
