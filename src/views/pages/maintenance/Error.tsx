import { Link, useLocation } from 'react-router-dom';
import { useEffect } from 'react';

// material-ui
import Button from '@mui/material/Button';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import Chip from '@mui/material/Chip';

// project imports
import { DASHBOARD_PATH } from 'config';
import AnimateButton from 'ui-component/extended/AnimateButton';
import { gridSpacing } from 'store/constant';

// assets
import HomeTwoToneIcon from '@mui/icons-material/HomeTwoTone';
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline';

// ==============================|| ERROR PAGE ||============================== //

export default function Error() {
  const location = useLocation();

  // Set document title to include 404 status
  useEffect(() => {
    document.title = '404 - Page Not Found';
  }, []);

  return (
    <Stack sx={{ gap: gridSpacing, alignItems: 'center', justifyContent: 'center' }}>
      {/*<Box sx={{ maxWidth: { xs: 350, sm: 580, md: 720 }, margin: '0 auto', position: 'relative' }}></Box>*/}
      <Stack spacing={gridSpacing} sx={{ justifyContent: 'center', alignItems: 'center', p: 1.5, maxWidth: 450 }}>
        <Chip label="404 Error" color="error" icon={<ErrorOutlineIcon />} sx={{ borderRadius: '16px', fontSize: '1rem', height: '32px' }} />
        <Typography variant="h1">Page Not Found</Typography>
        <Typography variant="body1" align="center">
          The page <strong>{location.pathname}</strong> you are looking for doesn't exist or might have been moved.
        </Typography>
        <AnimateButton>
          <Button variant="contained" size="large" component={Link} to={DASHBOARD_PATH}>
            <HomeTwoToneIcon sx={{ fontSize: '1.3rem', mr: 0.75 }} /> Return Home
          </Button>
        </AnimateButton>
      </Stack>
    </Stack>
  );
}
