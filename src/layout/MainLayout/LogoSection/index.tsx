import { ReactComponent as Cp0xLogo } from '@/assets/images/cp0x-logo.svg';
// material-ui
import Link from '@mui/material/Link';

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
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'flex-start',
        textDecoration: 'none',
        borderRadius: '4px'
      }}
    >
      <Cp0xLogo style={{ width: 50, height: 30 }} aria-hidden="true" focusable="false" />
    </Link>
  );
}
