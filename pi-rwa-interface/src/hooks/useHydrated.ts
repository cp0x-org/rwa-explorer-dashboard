import { useSyncExternalStore } from 'react';

const subscribe = () => () => {};

// false in the prerender and while React hydrates a prerendered page, true afterwards: for output that depends on the
// viewer (current time), so the hydrated markup matches the static HTML and changes right after.
export default function useHydrated(): boolean {
  return useSyncExternalStore(
    subscribe,
    () => true,
    () => false
  );
}
