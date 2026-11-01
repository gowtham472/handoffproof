import { test, expect } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { deleteUser, userByEmail, withDb } from './db';

test('registration: a new customer registers and the user row exists', async ({ request }) => {
  const email = 'reg-' + randomUUID() + '@example.test';
  let userId = 0;
  try {
    const res = await request.post('/register', { data: { email, name: 'Reg Test' } });
    expect(res.status()).toBe(201);
    await withDb(async (db) => {
      const row = await userByEmail(db, email);
      expect(row, 'user row must exist in the database').not.toBeNull();
      userId = row?.id ?? 0;
    });
    expect(userId).toBeGreaterThan(0);
  } finally {
    if (userId > 0) {
      await withDb((db) => deleteUser(db, userId));
    }
  }
});
