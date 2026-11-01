import { Pool } from 'pg';
import { readdir, readFile } from 'node:fs/promises';
import { join } from 'node:path';

const databaseUrl = process.env.DATABASE_URL;
if (!databaseUrl) {
  throw new Error('DATABASE_URL is required');
}

export const pool = new Pool({ connectionString: databaseUrl });

export async function migrate(migrationsDir: string): Promise<void> {
  const files = (await readdir(migrationsDir)).filter((f) => f.endsWith('.sql')).sort();
  for (const file of files) {
    const sql = await readFile(join(migrationsDir, file), 'utf8');
    await pool.query(sql);
  }
}
