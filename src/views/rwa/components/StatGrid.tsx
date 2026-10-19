import { Fragment, ReactNode } from 'react';
import { useTheme } from '@mui/material/styles';
import Box from '@mui/material/Box';
import Grid from '@mui/material/Grid';
import Link from '@mui/material/Link';
import Typography from '@mui/material/Typography';

import VisuallyHidden from 'ui-component/VisuallyHidden';
import { TokenIcon } from 'components/TokenIcon';
import { logoUrl } from 'api/rwa';
import { LocaleLink, useT } from 'i18n';
import HintTooltip from './HintTooltip';

export interface Stat {
  label: string;
  value: ReactNode;
  hint?: string;
}

// Metric cards as a description list: the label names the value; neither is a heading. The hint is a tooltip for the
// eye and hidden text for assistive technology.
export function StatGrid({ stats }: { stats: Stat[] }) {
  const theme = useTheme();
  // only the vertical <dl> margins are reset: the horizontal ones are the grid's own spacing compensation
  return (
    <Grid container component="dl" spacing={{ xs: 1, sm: 2 }} sx={{ marginTop: 0, marginBottom: 0 }}>
      {/* each card is one <div> group straight inside the <dl>: dt before dd in the markup (screen readers read
          "label, value"), the value shown above the label */}
      {stats.map((s) => (
        <Grid
          key={s.label}
          size={{ xs: 6, sm: 4, md: 3 }}
          sx={{ display: 'flex', flexDirection: 'column-reverse', gap: { xs: 0.5, sm: 1 }, padding: { xs: 1, sm: 2.5 } }}
        >
          <HintTooltip title={s.hint}>
            <Typography
              variant="h6"
              component="dt"
              sx={{ fontWeight: 400, color: theme.palette.grey[500], cursor: s.hint ? 'help' : 'default', textWrap: 'balance' }}
            >
              {s.label}
              {s.hint && <VisuallyHidden>. {s.hint}</VisuallyHidden>}
            </Typography>
          </HintTooltip>
          <Typography variant="h3" component="dd" sx={{ margin: 0 }}>
            {s.value}
          </Typography>
        </Grid>
      ))}
    </Grid>
  );
}

export interface Crumb {
  label: ReactNode;
  to: string; // locale-free path
}

interface DetailHeaderProps {
  crumbs: Crumb[]; // parent pages, in order; the title is the current page
  title: string;
  logo?: string | null;
  subtitle?: ReactNode;
  chips?: ReactNode;
  stats: Stat[];
}

// Same structure as the template's vault details header: breadcrumb, large title (the page's h1), stat cards.
export default function DetailHeader({ crumbs, title, logo, subtitle, chips, stats }: DetailHeaderProps) {
  const theme = useTheme();
  const t = useT();
  return (
    <Box
      sx={{
        padding: { xs: '4px 12px', sm: '10px 20px' },
        display: 'flex',
        flexDirection: 'column',
        marginBottom: { xs: 3, sm: 4 },
        width: '100%',
        backgroundColor: 'background.default',
        borderRadius: '12px'
      }}
    >
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: { xs: 1.5, sm: 2.5 },
          paddingTop: { xs: 1.5, sm: '20px' },
          marginBottom: { xs: 1, sm: 3 },
          flexWrap: 'wrap'
        }}
      >
        <TokenIcon symbol={title} src={logoUrl(logo)} avatarProps={{ sx: { width: 45, height: 45 } }} />
        <Box sx={{ minWidth: 0 }}>
          <Box component="nav" aria-label={t('common.breadcrumb')}>
            <Box component="ol" sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.75, listStyle: 'none', margin: 0, padding: 0 }}>
              {crumbs.map((c, i) => (
                <Fragment key={i}>
                  <Typography component="li" variant="h4" sx={{ fontSize: '16px', fontWeight: 600, color: theme.palette.grey[500] }}>
                    <Link component={LocaleLink} to={c.to} color="inherit" underline="hover">
                      {c.label}
                    </Link>
                  </Typography>
                  <Typography component="li" aria-hidden="true" sx={{ fontSize: '16px', color: theme.palette.grey[500] }}>
                    ›
                  </Typography>
                </Fragment>
              ))}
              <Typography
                component="li"
                variant="h4"
                aria-current="page"
                sx={{ fontSize: '16px', fontWeight: 600, color: theme.palette.grey[500] }}
              >
                {title}
              </Typography>
            </Box>
          </Box>
          <Typography variant="h2" component="h1" sx={{ display: 'block', wordBreak: 'break-word' }}>
            {title}
          </Typography>
          {subtitle && (
            <Typography variant="body2" color="text.secondary" component="div" sx={{ marginTop: 0.5 }}>
              {subtitle}
            </Typography>
          )}
        </Box>
        {chips && <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>{chips}</Box>}
      </Box>
      <StatGrid stats={stats} />
    </Box>
  );
}
