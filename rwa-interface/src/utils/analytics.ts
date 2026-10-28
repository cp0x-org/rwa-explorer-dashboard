// Google Analytics: gtag.js and its config are in index.html, off on localhost. The site is a single-page app, so gtag
// sends no page_view by itself (send_page_view: false): the app sends one per page it shows, the first one included.

declare global {
  interface Window {
    gtag?: (...args: unknown[]) => void;
  }
}

let lastPathname: string | undefined;

// Called once document.title is the title of the new page. A page counts once per path: the query string (the asset
// picked on the summary page) and a re-render with the same path send nothing.
export function trackPageView() {
  if (typeof window.gtag !== 'function') return;
  const { origin, pathname, search } = window.location;
  if (pathname === lastPathname) return;
  lastPathname = pathname;
  window.gtag('event', 'page_view', {
    page_path: pathname + search,
    page_title: document.title,
    page_location: origin + pathname + search
  });
}
