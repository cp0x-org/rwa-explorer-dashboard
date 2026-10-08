import { ReactNode } from 'react';
import { useTheme } from '@mui/material/styles';
import Box from '@mui/material/Box';
import Grid from '@mui/material/Grid';
import Stack from '@mui/material/Stack';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';

import SubCard from 'ui-component/cards/SubCard';
import { TokenIcon } from 'components/TokenIcon';
import { logoUrl } from 'api/rwa';

export interface Stat {
  label: string;
  value: ReactNode;
  hint?: string;
}

export function StatGrid({ stats }: { stats: Stat[] }) {
  const theme = useTheme();
  return (
    <Grid container spacing={2}>
      {stats.map((s) => (
        <Grid key={s.label} size={{ xs: 6, sm: 4, md: 3 }}>
          <SubCard sx={{ border: 'none', backgroundColor: 'transparent', ':hover': { boxShadow: 'none' } }}>
            <Stack spacing={1}>
              <Typography variant="h3">{s.value}</Typography>
              <Tooltip title={s.hint ?? ''} arrow disableHoverListener={!s.hint}>
                <Typography variant="h6" sx={{ fontWeight: 400, color: theme.palette.grey[500], cursor: s.hint ? 'help' : 'default' }}>
                  {s.label}
                </Typography>
              </Tooltip>
            </Stack>
          </SubCard>
        </Grid>
      ))}
    </Grid>
  );
}

interface DetailHeaderProps {
  caption: ReactNode;
  title: string;
  logo?: string | null;
  subtitle?: ReactNode;
  chips?: ReactNode;
  stats: Stat[];
}

// Same structure as the template's vault details header: caption, large title, stat cards.
export default function DetailHeader({ caption, title, logo, subtitle, chips, stats }: DetailHeaderProps) {
  const theme = useTheme();
  return (
    <Box
      sx={{
        padding: '10px 20px',
        display: 'flex',
        flexDirection: 'column',
        marginBottom: 4,
        width: '100%',
        backgroundColor: 'background.default',
        borderRadius: '12px'
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2.5, paddingTop: '20px', marginBottom: 3, flexWrap: 'wrap' }}>
        <TokenIcon symbol={title} src={logoUrl(logo)} avatarProps={{ sx: { width: 45, height: 45 } }} />
        <Box sx={{ minWidth: 0 }}>
          <Typography variant="h4" component="span" sx={{ display: 'block', fontSize: '16px', color: theme.palette.grey[500] }}>
            {caption}
          </Typography>
          <Typography variant="h2" component="span" sx={{ display: 'block', wordBreak: 'break-word' }}>
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
