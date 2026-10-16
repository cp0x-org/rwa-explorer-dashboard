import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';
import tsconfigPaths from 'vite-tsconfig-paths';
import svgr from 'vite-plugin-svgr';

export default defineConfig(({ mode, isSsrBuild }) => {
  // depending on your application, base can also be "/"
  const env = loadEnv(mode, process.cwd(), '');
  // const API_URL = `${env.VITE_APP_BASE_NAME}`;
  const API_URL = '/';
  const PORT = 3000;

  return {
    server: {
      // this ensures that the browser opens upon server start
      open: true,
      // this sets a default port to 3000
      port: PORT,
      host: true
    },
    build: {
      chunkSizeWarningLimit: 1600,
      // `vite build --ssr src/entry-server.tsx`: a Node module for scripts/prerender.mjs, without a copy of public/
      ...(isSsrBuild && {
        copyPublicDir: false,
        rollupOptions: { output: { entryFileNames: '[name].mjs', chunkFileNames: '[name]-[hash].mjs' } }
      })
    },
    ssr: {
      // bundle the dependencies too: Node then needs no ESM/CJS interop for MUI, icons and the rest
      noExternal: true
    },
    preview: {
      open: true,
      host: true
    },
    define: isSsrBuild ? {} : { global: 'window' },
    resolve: {
      alias: {
        // { find: '', replacement: path.resolve(__dirname, 'src') },
        // {
        //   find: /^~(.+)/,
        //   replacement: path.join(process.cwd(), 'node_modules/$1')
        // },
        // {
        //   find: /^src(.+)/,
        //   replacement: path.join(process.cwd(), 'src/$1')
        // }
        // {
        //   find: 'assets',
        //   replacement: path.join(process.cwd(), 'src/assets')
        // },
        '@tabler/icons-react': '@tabler/icons-react/dist/esm/icons/index.mjs'
      }
    },
    base: API_URL,
    plugins: [
      react(),
      tsconfigPaths(),
      svgr({
        svgrOptions: {
          icon: true,
          exportType: 'named',
        },
        include: '**/*.svg',
      })
    ]
  };
});
