import { ReactNode } from 'react';

// project imports
import Locales from 'ui-component/Locales';
// import RTLLayout from 'ui-component/RTLLayout';
import Snackbar from 'ui-component/extended/Snackbar';
import Notistack from 'ui-component/third-party/Notistack';

import ThemeCustomization from 'themes';

// ==============================|| APP ||============================== //

// `router` is the router provider: the browser one in index.tsx, a static one in the prerender (entry-server.tsx).
export default function App({ router }: { router: ReactNode }) {
  return (
    <ThemeCustomization>
      {/* <RTLLayout> */}
      <Locales>
        <Notistack>
          {router}
          <Snackbar />
        </Notistack>
      </Locales>
      {/* </RTLLayout> */}
    </ThemeCustomization>
  );
}
