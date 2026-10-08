import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';

import { RwaExport } from 'types/rwa';
import { fmtAge, fmtDate, fmtUsd } from 'utils/rwa';

// Update time and counting method, always next to the data.
export default function MethodNote({ data }: { data: RwaExport }) {
  const { oldest_fetched_at: oldest, newest_fetched_at: newest } = data.summary;
  const s = data.settings;
  const method =
    `Rating = number of eligible DeFi positions that contain the token, confirmed by its contract address in the position ` +
    `composition (directly or via LP → vault, up to ${s.max_graph_depth} levels). Eligible: real use-case (LP, lending supply, ` +
    `vault, staking, yield market), liquidity ≥ ${fmtUsd(s.min_eligible_liquidity_usd)}, source data ≤ ${s.max_position_age_hours}h old ` +
    `at collection. APY is the reported APY of one position, not an expected return. Only positions indexed by Portals and ` +
    `found by search are covered; unsupported networks mean unknown usage, not zero.`;

  return (
    <Typography variant="body2" color="text.secondary" sx={{ marginBottom: 2 }}>
      Data collected {oldest && oldest !== newest ? `${fmtDate(oldest)} – ` : ''}
      {fmtDate(newest)} ({fmtAge(newest)}) · Sources: CoinMarketCap, Portals · Coverage: search ·{' '}
      <Tooltip title={method} arrow>
        <span style={{ textDecoration: 'underline dotted', cursor: 'help' }}>How the rating works</span>
      </Tooltip>
    </Typography>
  );
}
