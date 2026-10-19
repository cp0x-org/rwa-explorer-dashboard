// material-ui
import { Box } from '@mui/material';

import AppLogo from 'assets/images/cp0x-logo.svg';

export default function Logo() {
  return (
    <Box display="flex" flexDirection="column" alignItems="center" gap={1}>
      <img src={AppLogo} alt="logo" width={40} />
    </Box>
  );
}
