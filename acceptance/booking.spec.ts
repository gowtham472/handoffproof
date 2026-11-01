import { test, expect } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { bookingById, deleteUser, userByEmail, withDb } from './db';

test('booking: a customer books and the booking row exists', async ({ request }) => {
  const email = 'book-' + randomUUID() + '@example.test';
  let userId = 0;
  try {
    const reg = await request.post('/register', { data: { email, name: 'Book Test' } });
    expect(reg.status()).toBe(201);
    await withDb(async (db) => {
      userId = (await userByEmail(db, email))?.id ?? 0;
    });
    expect(userId).toBeGreaterThan(0);

    const res = await request.post('/bookings', { data: { user_id: userId, service: 'checkup' } });
    expect(res.status()).toBe(201);
    const body = (await res.json()) as { id: number; status: string };
    expect(body.status).toBe('confirmed');

    await withDb(async (db) => {
      const row = await bookingById(db, body.id);
      expect(row, 'booking row must exist in the database').not.toBeNull();
      expect(row?.user_id).toBe(userId);
    });
  } finally {
    if (userId > 0) {
      await withDb((db) => deleteUser(db, userId));
    }
  }
});
