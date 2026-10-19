import { ReactComponent as Cp0xLogo } from '@/assets/images/cp0x-logo.svg';
// material-ui
import Link from '@mui/material/Link';
import Typography from '@mui/material/Typography';

// project imports
import { DASHBOARD_PATH } from 'config';
import { LocaleLink, useT } from 'i18n';

// ==============================|| MAIN LOGO ||============================== //

export default function LogoSection() {
  const t = useT();
  return (
    <Link
      component={LocaleLink}
      to={DASHBOARD_PATH}
      aria-label={t('header.logo')}
      sx={{
        display: 'flex',
        alignItems: 'center',
        gap: 1.25,
        textDecoration: 'none',
        color: 'text.primary',
        borderRadius: '4px'
      }}
    >
      <Cp0xLogo style={{ width: 50, height: 30, flexShrink: 0 }} aria-hidden="true" focusable="false" />
      {/* the site title next to the logo (decorative for the link: its name says the same) */}
      <Typography
        component="span"
        aria-hidden="true"
        sx={{ fontSize: '17px', fontWeight: 600, letterSpacing: '0.01em', whiteSpace: 'nowrap' }}
      >
        {t('site.title')}
      </Typography>
    </Link>
  );
}
