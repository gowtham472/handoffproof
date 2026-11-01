'use strict';
// Local mail provider mock (hp-build-mail-sink).
// Validates a Bearer token on every send so token withdrawal is deterministic.
// State is in memory and reset between drills via POST /admin/reset.
const http = require('node:http');
const { createHash, randomUUID, timingSafeEqual } = require('node:crypto');

const MAX_BODY = 1024 * 1024;
const MAX_MESSAGES = 1000;

function safeEqual(a, b) {
  const ba = Buffer.from(String(a));
  const bb = Buffer.from(String(b));
  return ba.length === bb.length && ba.length > 0 && timingSafeEqual(ba, bb);
}

function fingerprint(token) {
  if (!token) return 'none';
  return createHash('sha256').update(token).digest('hex').slice(0, 8);
}

function initialTokens() {
  const list = process.env.MAIL_SINK_VALID_TOKENS || process.env.MAIL_API_TOKEN || '';
  return new Set(list.split(',').map((s) => s.trim()).filter(Boolean));
}

function createServer() {
  const adminToken = process.env.MAIL_SINK_ADMIN_TOKEN || '';
  let validTokens = initialTokens();
  let messages = [];

  function bearer(req) {
    const h = req.headers.authorization || '';
    return h.startsWith('Bearer ') ? h.slice(7) : '';
  }
  function isAdmin(req) {
    return safeEqual(bearer(req), adminToken);
  }
  function canRead(req) {
    if (process.env.MAIL_SINK_REQUIRE_AUTH !== '1') return true;
    return isAdmin(req);
  }
  function sendJson(res, status, obj) {
    const body = JSON.stringify(obj);
    res.writeHead(status, { 'content-type': 'application/json' });
    res.end(body);
  }
  function readBody(req) {
    return new Promise((resolve, reject) => {
      const chunks = [];
      let size = 0;
      req.on('data', (c) => {
        size += c.length;
        if (size > MAX_BODY) {
          reject(new Error('body_too_large'));
          req.destroy();
          return;
        }
        chunks.push(c);
      });
      req.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')));
      req.on('error', reject);
    });
  }

  const server = http.createServer(async (req, res) => {
    const url = new URL(req.url || '/', 'http://localhost');
    const path = url.pathname;
    let status = 404;
    try {
      if (req.method === 'GET' && path === '/health') {
        status = 200;
        return sendJson(res, status, { status: 'ok' });
      }
      if (req.method === 'POST' && path === '/send') {
        const token = bearer(req);
        if (!validTokens.has(token)) {
          status = 401;
          return sendJson(res, status, { error: 'invalid_token' });
        }
        const raw = await readBody(req);
        let payload;
        try {
          payload = raw ? JSON.parse(raw) : {};
        } catch {
          status = 400;
          return sendJson(res, status, { error: 'invalid_json' });
        }
        if (typeof payload.to !== 'string' || payload.to.length === 0) {
          status = 400;
          return sendJson(res, status, { error: 'missing_to' });
        }
        const msg = {
          id: randomUUID(),
          to: payload.to,
          subject: typeof payload.subject === 'string' ? payload.subject : '',
          body: typeof payload.body === 'string' ? payload.body : '',
          received_at: new Date().toISOString(),
        };
        messages.push(msg);
        if (messages.length > MAX_MESSAGES) messages.splice(0, messages.length - MAX_MESSAGES);
        status = 200;
        return sendJson(res, status, { stored: true, id: msg.id });
      }
      if (req.method === 'GET' && path === '/messages') {
        if (!canRead(req)) {
          status = 401;
          return sendJson(res, status, { error: 'unauthorized' });
        }
        const to = url.searchParams.get('to');
        const list = to ? messages.filter((m) => m.to === to) : messages;
        status = 200;
        return sendJson(res, status, { messages: list });
      }
      if (req.method === 'POST' && (path === '/admin/revoke' || path === '/admin/reset')) {
        if (!isAdmin(req)) {
          status = 403;
          return sendJson(res, status, { error: 'forbidden' });
        }
        if (path === '/admin/reset') {
          messages = [];
          validTokens = initialTokens();
          status = 200;
          return sendJson(res, status, { reset: true });
        }
        const raw = await readBody(req);
        let payload = {};
        try {
          payload = raw ? JSON.parse(raw) : {};
        } catch {
          status = 400;
          return sendJson(res, status, { error: 'invalid_json' });
        }
        const removed = typeof payload.token === 'string' ? validTokens.delete(payload.token) : false;
        status = 200;
        return sendJson(res, status, { revoked: removed });
      }
      status = 404;
      return sendJson(res, status, { error: 'not_found' });
    } catch {
      status = 500;
      return sendJson(res, status, { error: 'internal' });
    } finally {
      console.log(req.method + ' ' + path + ' ' + status + ' token_fp=' + fingerprint(bearer(req)));
    }
  });
  return server;
}

if (require.main === module) {
  const port = Number(process.env.MAIL_SINK_PORT || 4100);
  createServer().listen(port, () => console.log('mail-sink listening on ' + port));
}

module.exports = { createServer };
