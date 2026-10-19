import Chip from '@mui/material/Chip';

import VisuallyHidden from 'ui-component/VisuallyHidden';
import { coverageChip, deploymentChip, eligibilityChip } from 'utils/rwa';
import { useT } from 'i18n';
import HintTooltip from './HintTooltip';

// A status chip whose explanation is a tooltip for the eye and hidden text for assistive technology.
function HintChip({ label, color, hint }: { label: string; color: Parameters<typeof Chip>[0]['color']; hint: string }) {
  return (
    <HintTooltip title={hint}>
      <Chip
        size="small"
        color={color}
        label={
          <>
            {label}
            {hint && <VisuallyHidden>: {hint}</VisuallyHidden>}
          </>
        }
      />
    </HintTooltip>
  );
}

export function CoverageChip({ coverage, rawCount }: { coverage: string; rawCount?: number }) {
  const t = useT();
  if (coverage === 'search') {
    if (rawCount !== 0) return null;
    return <HintChip label={t('coverage.search.noPositions')} color="default" hint={t('coverage.search.noPositions.hint')} />;
  }
  const c = coverageChip(t, coverage);
  return <HintChip label={c.label} color={c.color} hint={c.hint} />;
}

export function DeploymentChip({ status }: { status: string }) {
  const t = useT();
  const c = deploymentChip(t, status);
  return <Chip size="small" label={c.label} color={c.color} />;
}

export function EligibilityChip({ eligibility }: { eligibility: string }) {
  const t = useT();
  const c = eligibilityChip(t, eligibility);
  return <HintChip label={c.label} color={c.color} hint={c.hint} />;
}
