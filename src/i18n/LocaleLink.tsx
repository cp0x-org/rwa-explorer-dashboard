import { ComponentProps, Ref } from 'react';
import { Link as RouterLink, NavigateOptions, To, useNavigate } from 'react-router-dom';

import { useLocale } from './LocaleProvider';

const localizeTo = (to: To, path: (to: string) => string): To => {
  if (typeof to === 'string') return path(to);
  return to.pathname ? { ...to, pathname: path(to.pathname) } : to;
};

type LocaleLinkProps = ComponentProps<typeof RouterLink> & { ref?: Ref<HTMLAnchorElement> };

// React Router's Link with the locale prefix of the current page: `to="/assets/NVDA"` renders `/zh/assets/NVDA` on
// a Chinese page. Views only ever use locale-free paths.
export function LocaleLink({ to, ref, ...props }: LocaleLinkProps) {
  const { path } = useLocale();
  return <RouterLink ref={ref} to={localizeTo(to, path)} {...props} />;
}

// `navigate('/tokens/1')` within the current locale.
export function useLocaleNavigate() {
  const navigate = useNavigate();
  const { path } = useLocale();
  return (to: To, options?: NavigateOptions) => navigate(localizeTo(to, path), options);
}
