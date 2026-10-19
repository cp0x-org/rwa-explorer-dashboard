import { defineConfig } from 'vitest/config';
import tsconfigPaths from 'vite-tsconfig-paths';

// Unit tests: dictionaries, page metadata, locale paths, SEO helpers. E2E tests live in e2e/ (Playwright).
export default defineConfig({
  plugins: [tsconfigPaths()],
  test: {
    include: ['src/**/*.test.ts'],
    environment: 'node'
  }
});
