// material-ui
import Box from '@mui/material/Box';
import Link from '@mui/material/Link';

// project imports
import LogoSection from '../LogoSection';
import LanguageSwitch from './LanguageSwitch';
import MenuItems from './MenuItems';
import MobileMenu from './MobileMenu';
import { visuallyHidden } from 'ui-component/VisuallyHidden';
import { useT } from 'i18n';

// ==============================|| MAIN NAVBAR / HEADER ||============================== //

export default function Header() {
  const t = useT();
  return (
    <>
      {/* keyboard users jump over the header; the link appears when focused */}
      <Link
        href="#main"
        sx={{
          ...visuallyHidden,
          '&:focus': {
            position: 'fixed',
            top: 8,
            left: 8,
            width: 'auto',
            height: 'auto',
            margin: 0,
            padding: 1,
            clip: 'auto',
            zIndex: 2000,
            bgcolor: 'background.paper'
          }
        }}
      >
        {t('site.skipToContent')}
      </Link>

      {/* logo on the left, the site menu starts a little to the right of it */}
      <Box sx={{ display: 'flex', alignItems: 'center', ml: { xs: 0, md: 1.25 } }}>
        <LogoSection />
      </Box>

      {/* site menu (desktop) */}
      <MenuItems />

      {/* language, mobile menu: top right */}
      <Box sx={{ flexGrow: 1, display: 'flex', justifyContent: 'flex-end', alignItems: 'center', gap: { xs: 0.5, sm: 1.5 } }}>
        <LanguageSwitch />
        <MobileMenu />
      </Box>
    </>
  );
}
