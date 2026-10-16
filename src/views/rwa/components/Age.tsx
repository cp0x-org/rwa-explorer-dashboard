import useHydrated from 'hooks/useHydrated';
import { fmtAge, fmtDate } from 'utils/rwa';

// "2d ago" depends on the viewer's clock: the prerendered HTML shows the UTC time instead, the browser switches to the age.
export default function Age({ iso }: { iso: string | null | undefined }) {
  const hydrated = useHydrated();
  return <>{hydrated ? fmtAge(iso) : fmtDate(iso)}</>;
}
