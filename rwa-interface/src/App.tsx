import { ReactNode } from 'react';

// project imports
import Snackbar from 'ui-component/extended/Snackbar';
import Notistack from 'ui-component/third-party/Notistack';

import ThemeCustomization from 'themes';

// ==============================|| APP ||============================== //

// `router` is the router provider: the browser one in index.tsx, a static one in the prerender (entry-server.tsx).
// The language provider (react-intl) sits inside the route tree, because the language comes from the URL.
export default function App({ router }: { router: ReactNode }) {
  return (
    <ThemeCustomization>
      <Notistack>
        {router}
        <Snackbar />
      </Notistack>
    </ThemeCustomization>
  );
}
