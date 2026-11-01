import { test, expect } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { deleteUser, userByEmail, withDb } from './db';

function sinkUrl(): string {
  return process.env.HANDOFFPROOF_MAIL_SINK_URL ?? 'http://localhost:4100';
}

// Independent evidence: the sink, never the app's own success message.
// This is the test that catches a silent email failure.
test('confirmation email: after booking the sink holds exactly one message', async ({ request }) => {
  const email = 'mail-' + randomUUID() + '@example.test';
  let userId = 0;
  try {
    const reg = await request.post('/register', { data: { email, name: 'Mail Test' } });
    expect(reg.status()).toBe(201);
    await withDb(async (db) => {
      userId = (await userByEmail(db, email))?.id ?? 0;
    });
    expect(userId).toBeGreaterThan(0);

    const res = await request.post('/bookings', { data: { user_id: userId, service: 'checkup' } });
    expect(res.status()).toBe(201);

    await expect
      .poll(
        async () => {
          const r = await request.get(
            sinkUrl() + '/messages?to=' + encodeURIComponent(email),
          );
          if (!r.ok()) return -1;
          const body = (await r.json()) as { messages: unknown[] };
          return body.messages.length;
        },
        { timeout: 10000 },
      )
      .toBe(1);
  } finally {
    if (userId > 0) {
      await withDb((db) => deleteUser(db, userId));
    }
  }
});
