import { Client } from 'pg';

export function dbUrl(): string {
  return (
    process.env.HANDOFFPROOF_DATABASE_URL ??
    'postgres://handoff:handoff_pw@localhost:55432/handoffproof'
  );
}

export async function withDb<T>(fn: (db: Client) => Promise<T>): Promise<T> {
  const db = new Client({ connectionString: dbUrl() });
  await db.connect();
  try {
    return await fn(db);
  } finally {
    await db.end();
  }
}

export async function userByEmail(db: Client, email: string): Promise<{ id: number } | null> {
  const r = await db.query('SELECT id FROM users WHERE email = $1', [email]);
  if (r.rowCount === 0) return null;
  return { id: Number(r.rows[0].id) };
}

export async function bookingById(
  db: Client,
  id: number,
): Promise<{ id: number; user_id: number } | null> {
  const r = await db.query('SELECT id, user_id FROM bookings WHERE id = $1', [id]);
  if (r.rowCount === 0) return null;
  return { id: Number(r.rows[0].id), user_id: Number(r.rows[0].user_id) };
}

export async function documentById(
  db: Client,
  id: string,
): Promise<{ id: string; sha256: string } | null> {
  const r = await db.query('SELECT id, sha256 FROM documents WHERE id = $1', [id]);
  if (r.rowCount === 0) return null;
  return { id: String(r.rows[0].id), sha256: String(r.rows[0].sha256) };
}

export async function deleteUser(db: Client, userId: number): Promise<void> {
  await db.query('DELETE FROM bookings WHERE user_id = $1', [userId]);
  await db.query('DELETE FROM users WHERE id = $1', [userId]);
}

export async function deleteDocument(db: Client, id: string): Promise<void> {
  await db.query('DELETE FROM documents WHERE id = $1', [id]);
}
