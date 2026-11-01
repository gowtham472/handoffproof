import express, { Request, Response } from 'express';
import { createHash, randomUUID } from 'node:crypto';
import { mkdir, stat, writeFile } from 'node:fs/promises';
import { basename, join } from 'node:path';
import { migrate, pool } from './db';
import { sendMail } from './mail';
import { slugify } from './utils/notes';

const app = express();
app.use(express.json({ limit: '1mb' }));

const UPLOAD_DIR = '/data/uploads';

function errMsg(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

app.post('/register', async (req: Request, res: Response) => {
  const email = typeof req.body?.email === 'string' ? req.body.email.trim() : '';
  const name = typeof req.body?.name === 'string' ? req.body.name.trim() : '';
  if (!email || !name || email.length > 254 || name.length > 100 || !/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email)) {
    res.status(400).json({ error: 'email and name are required' });
    return;
  }
  try {
    const r = await pool.query(
      'INSERT INTO users (email, name) VALUES ($1, $2) RETURNING id, email, name',
      [email, name],
    );
    res.status(201).json(r.rows[0]);
  } catch (err) {
    if (errMsg(err).includes('duplicate key')) {
      res.status(409).json({ error: 'email already registered' });
      return;
    }
    console.error('register failed: ' + errMsg(err));
    res.status(500).json({ error: 'register failed' });
  }
});

app.post('/bookings', async (req: Request, res: Response) => {
  const userId = Number(req.body?.user_id);
  const service = typeof req.body?.service === 'string' ? req.body.service.trim() : '';
  const startsAt = typeof req.body?.starts_at === 'string' ? req.body.starts_at : null;
  if (!Number.isInteger(userId) || !service) {
    res.status(400).json({ error: 'user_id and service are required' });
    return;
  }
  let bookingId = 0;
  let email = '';
  try {
    const u = await pool.query('SELECT email FROM users WHERE id = $1', [userId]);
    if (u.rowCount === 0) {
      res.status(404).json({ error: 'user not found' });
      return;
    }
    email = String(u.rows[0].email);
    const b = await pool.query(
      'INSERT INTO bookings (user_id, service, starts_at) VALUES ($1, $2, $3) RETURNING id',
      [userId, service, startsAt],
    );
    bookingId = Number(b.rows[0].id);
  } catch (err) {
    console.error('booking failed: ' + errMsg(err));
    res.status(500).json({ error: 'booking failed' });
    return;
  }
  try {
    await sendMail(email, 'Booking confirmed', 'Your booking #' + bookingId + ' for ' + service + ' is confirmed.');
  } catch (err) {
    console.error('confirmation email failed for booking ' + bookingId + ': ' + errMsg(err));
  }
  res.status(201).json({ id: bookingId, status: 'confirmed' });
});

app.post('/uploads', async (req: Request, res: Response) => {
  const name = typeof req.body?.name === 'string' ? req.body.name : '';
  const b64 = typeof req.body?.content_base64 === 'string' ? req.body.content_base64 : '';
  if (!name || !b64) {
    res.status(400).json({ error: 'name and content_base64 are required' });
    return;
  }
  const data = Buffer.from(b64, 'base64');
  if (data.length > 1024 * 1024) {
    res.status(413).json({ error: 'file too large (1MB max)' });
    return;
  }
  const sha256 = createHash('sha256').update(data).digest('hex');
  const id = randomUUID();
  const storedName = id + '-' + slugify(basename(name));
  try {
    await mkdir(UPLOAD_DIR, { recursive: true });
    await writeFile(join(UPLOAD_DIR, storedName), data);
    await pool.query(
      'INSERT INTO documents (id, filename, sha256, size_bytes) VALUES ($1, $2, $3, $4)',
      [id, name, sha256, data.length],
    );
    res.status(201).json({ id, filename: name, sha256, size_bytes: data.length });
  } catch (err) {
    console.error('upload failed: ' + errMsg(err));
    res.status(500).json({ error: 'upload failed' });
  }
});

app.get('/documents/:id', async (req: Request, res: Response) => {
  try {
    const r = await pool.query(
      'SELECT id, filename, sha256, size_bytes FROM documents WHERE id = $1',
      [req.params.id],
    );
    if (r.rowCount === 0) {
      res.status(404).json({ error: 'document not found' });
      return;
    }
    const row = r.rows[0] as { id: string; filename: string; sha256: string; size_bytes: number };
    let stored = false;
    try {
      const entries = await stat(join(UPLOAD_DIR, row.id + '-' + slugify(basename(row.filename))));
      stored = entries.isFile();
    } catch {
      stored = false;
    }
    res.json({ ...row, stored });
  } catch (err) {
    console.error('document lookup failed: ' + errMsg(err));
    res.status(500).json({ error: 'document lookup failed' });
  }
});

app.get('/health', async (_req: Request, res: Response) => {
  try {
    await pool.query('SELECT 1');
    res.json({ status: 'ok' });
  } catch (err) {
    console.error('health check failed: ' + errMsg(err));
    res.status(500).json({ status: 'error' });
  }
});

function startReminderJob(): void {
  const token = process.env.SCHEDULER_TOKEN || '';
  if (!token) {
    console.warn('SCHEDULER_TOKEN is not set; reminder job is disabled');
    return;
  }
  const timer = setInterval(() => {
    console.log('reminder job heartbeat');
  }, 30000);
  timer.unref();
  console.log('reminder job started');
}

async function main(): Promise<void> {
  const migrationsDir = join(process.cwd(), 'migrations');
  // The database may still be initialising on first boot; retry briefly.
  for (let attempt = 1; ; attempt++) {
    try {
      await migrate(migrationsDir);
      break;
    } catch (err) {
      if (attempt >= 15) throw err;
      console.warn('migration attempt ' + attempt + ' failed, retrying: ' + errMsg(err));
      await new Promise((r) => setTimeout(r, 2000));
    }
  }
  await mkdir(UPLOAD_DIR, { recursive: true });
  startReminderJob();
  const port = Number(process.env.PORT || 3000);
  app.listen(port, () => console.log('demo-app listening on ' + port));
}

main().catch((err: unknown) => {
  console.error('fatal: ' + errMsg(err));
  process.exit(1);
});
