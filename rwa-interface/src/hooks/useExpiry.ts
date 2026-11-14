import { useEffect, useState } from 'react';

type RecheckDelay = (asOf: string | null | undefined, validDays: number | null | undefined, now: Date) => number | null;

// Time-dependent output starts only after hydration. Re-arm capped timers and recheck tabs waking from sleep.
export default function useExpiry(
  asOf: string | null | undefined,
  validDays: number | null | undefined,
  recheckDelay: RecheckDelay
): Date | null {
  const [now, setNow] = useState<Date | null>(null);
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    const check = () => {
      clearTimeout(timer);
      const current = new Date();
      setNow(current);
      const delay = recheckDelay(asOf, validDays, current);
      if (delay !== null) timer = setTimeout(check, delay);
    };
    check();
    document.addEventListener('visibilitychange', check);
    window.addEventListener('pageshow', check);
    return () => {
      clearTimeout(timer);
      document.removeEventListener('visibilitychange', check);
      window.removeEventListener('pageshow', check);
    };
  }, [asOf, validDays, recheckDelay]);
  return now;
}
