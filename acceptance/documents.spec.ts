import { test, expect } from '@playwright/test';
import { createHash, randomUUID } from 'node:crypto';
import { deleteDocument, documentById, withDb } from './db';

// Recovery support: an uploaded file exists in storage and its checksum matches.
test('documents: upload persists with a matching checksum', async ({ request }) => {
  const content = Buffer.from('handoff-document-' + randomUUID(), 'utf8');
  const wantSha = createHash('sha256').update(content).digest('hex');
  let docId = '';
  try {
    const res = await request.post('/uploads', {
      data: { name: 'intake.txt', content_base64: content.toString('base64') },
    });
    expect(res.status()).toBe(201);
    const body = (await res.json()) as { id: string; sha256: string };
    docId = body.id;
    expect(body.sha256).toBe(wantSha);

    await withDb(async (db) => {
      const row = await documentById(db, docId);
      expect(row, 'document row must exist in the database').not.toBeNull();
      expect(row?.sha256).toBe(wantSha);
    });

    const got = await request.get('/documents/' + docId);
    expect(got.ok()).toBeTruthy();
    const detail = (await got.json()) as { sha256: string; stored: boolean };
    expect(detail.sha256).toBe(wantSha);
    expect(detail.stored, 'file must exist in upload storage').toBe(true);
  } finally {
    if (docId) {
      await withDb((db) => deleteDocument(db, docId));
    }
  }
});
