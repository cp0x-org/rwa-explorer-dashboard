import { useState } from 'react';

// material-ui
import { Box, IconButton, Drawer, List, ListItemButton, ListItemText, Typography, useTheme } from '@mui/material';
import { IconMenu2, IconX } from '@tabler/icons-react';

// project imports
import { LocaleLink, useT } from 'i18n';

const TITLE_ID = 'mobile-menu-title';

export default function MobileMenu() {
  const theme = useTheme();
  const t = useT();
  const [open, setOpen] = useState(false);

  const toggle = () => setOpen(!open);

  return (
    <Box sx={{ display: { xs: 'block', md: 'none' } }}>
      <IconButton
        color="inherit"
        onClick={toggle}
        edge="end"
        size="large"
        aria-label={t('header.openMenu')}
        aria-haspopup="dialog"
        aria-expanded={open}
      >
        <IconMenu2 aria-hidden="true" />
      </IconButton>

      <Drawer
        anchor="right"
        open={open}
        onClose={toggle}
        slotProps={{
          paper: {
            role: 'dialog',
            'aria-modal': true,
            'aria-labelledby': TITLE_ID,
            sx: { width: '280px', background: theme.palette.background.default }
          }
        }}
      >
        <Box sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Typography variant="h6" component="h2" id={TITLE_ID}>
            {t('header.menu')}
          </Typography>
          <IconButton color="inherit" onClick={toggle} edge="end" size="small" aria-label={t('header.closeMenu')}>
            <IconX aria-hidden="true" />
          </IconButton>
        </Box>

        <List component="nav" aria-label={t('header.siteMenu')} sx={{ px: 2, pt: 1 }}>
          <ListItemButton component={LocaleLink} to="/" onClick={toggle}>
            <ListItemText primary={t('header.home')} />
          </ListItemButton>
          <ListItemButton component="a" href="https://pi.cp0x.com" rel="noopener noreferrer" onClick={toggle}>
            <ListItemText primary={t('header.pi')} />
          </ListItemButton>
          <ListItemButton component="a" href="https://cp0x.com" target="_blank" rel="noopener noreferrer" onClick={toggle}>
            <ListItemText primary={t('header.referrals')} />
          </ListItemButton>
        </List>
      </Drawer>
    </Box>
  );
}
