import { useSyncExternalStore } from 'react';

const subscribe = () => () => {};

// false in the server render and while React hydrates a server-rendered page, true afterwards: for output that depends on the
// viewer (current time), so the hydrated markup matches the static HTML and changes right after.
export default function useHydrated(): boolean {
  return useSyncExternalStore(
    subscribe,
    () => true,
    () => false
  );
}
