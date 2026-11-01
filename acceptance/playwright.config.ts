import { defineConfig } from '@playwright/test';

// API-only acceptance tests (user decision 2026-11-01): request fixture plus
// direct DB and mail-sink assertions. No browsers are downloaded or launched.
export default defineConfig({
  testDir: '.',
  testMatch: '*.spec.ts',
  workers: 1,
  timeout: 30000,
  use: {
    baseURL: process.env.HANDOFFPROOF_BASE_URL ?? 'http://localhost:3000',
  },
});
