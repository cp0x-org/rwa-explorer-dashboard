// routes
import { localeRoute } from './MainRoutes';

// ==============================|| ROUTING RENDER ||============================== //

// One route tree for the browser router (index.tsx) and the build-time prerender (entry-server.tsx).
// `/zh` goes first so that it wins over the catch-all of the English tree.
const routes = [localeRoute('zh-CN'), localeRoute('en')];

export default routes;
