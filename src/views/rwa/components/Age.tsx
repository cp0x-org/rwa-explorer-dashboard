import useHydrated from 'hooks/useHydrated';
import { useFormat } from 'i18n';

// "2d ago" depends on the viewer's clock: the prerendered HTML shows the UTC time instead, the browser switches to the
// age (and keeps the exact time in the title and for assistive technology).
export default function Age({ iso }: { iso: string | null | undefined }) {
  const hydrated = useHydrated();
  const f = useFormat();
  if (!iso) return <>{f.date(iso)}</>;
  return hydrated ? (
    <time dateTime={iso} title={f.date(iso)}>
      {f.age(iso)}
    </time>
  ) : (
    <time dateTime={iso}>{f.date(iso)}</time>
  );
}
