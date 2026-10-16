// material-ui
import Box from '@mui/material/Box';

// project imports
import LogoSection from '../LogoSection';
import HeaderMenu from './HeaderMenu';

// ==============================|| MAIN NAVBAR / HEADER ||============================== //

export default function Header() {
  return (
    <>
      {/* logo & toggler button */}
      <Box sx={{ width: { xs: 'auto', md: 228 }, display: 'flex' }}>
        <Box component="span" sx={{ display: { xs: 'block', md: 'block' }, flexGrow: 1 }}>
          <LogoSection />
        </Box>
      </Box>

      {/*menu */}
      <Box sx={{ flexGrow: 1, display: 'flex', justifyContent: { xs: 'flex-end', md: 'flex-start' } }}>
        <HeaderMenu />
      </Box>
    </>
  );
}
