import Chip from '@mui/material/Chip';
import Tooltip from '@mui/material/Tooltip';

import { COVERAGE, DEPLOYMENT_STATUS, ELIGIBILITY } from 'utils/rwa';

export function CoverageChip({ coverage, rawCount }: { coverage: string; rawCount?: number }) {
  if (coverage === 'search') {
    if (rawCount !== 0) return null;
    return (
      <Tooltip title="No confirmed positions in current coverage: Portals search found no position containing this token's address" arrow>
        <Chip size="small" label="No positions" />
      </Tooltip>
    );
  }
  const c = COVERAGE[coverage] ?? { label: coverage, color: 'default', hint: '' };
  return (
    <Tooltip title={c.hint} arrow>
      <Chip size="small" label={c.label} color={c.color} />
    </Tooltip>
  );
}

export function DeploymentChip({ status }: { status: string }) {
  const c = DEPLOYMENT_STATUS[status] ?? { label: status, color: 'default' };
  return <Chip size="small" label={c.label} color={c.color} />;
}

export function EligibilityChip({ eligibility }: { eligibility: string }) {
  const c = ELIGIBILITY[eligibility] ?? { label: eligibility, color: 'default', hint: '' };
  return (
    <Tooltip title={c.hint} arrow>
      <Chip size="small" label={c.label} color={c.color} />
    </Tooltip>
  );
}
