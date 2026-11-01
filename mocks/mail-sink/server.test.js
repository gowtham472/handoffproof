'use strict';
const { test, before, after } = require('node:test');
const assert = require('node:assert/strict');
const { createServer } = require('./server');

const ADMIN = 'test-admin-token';
const TOKEN = 'test-mail-token';
let server;
let base;

before(async () => {
  process.env.MAIL_SINK_ADMIN_TOKEN = ADMIN;
  process.env.MAIL_SINK_VALID_TOKENS = TOKEN;
  server = createServer();
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  base = 'http://127.0.0.1:' + server.address().port;
});
after(async () => {
  await new Promise((resolve) => server.close(resolve));
});

async function req(method, path, token, body) {
  const headers = {};
  if (token) headers.authorization = 'Bearer ' + token;
  if (body !== undefined) headers['content-type'] = 'application/json';
  const res = await fetch(base + path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  return { status: res.status, json: text ? JSON.parse(text) : {} };
}

test('send OK, revoke, then 401 and count unchanged', async () => {
  const to = 'cust@example.test';
  let r = await req('POST', '/send', TOKEN, { to, subject: 'Booking confirmed', body: 'Hello' });
  assert.equal(r.status, 200);

  r = await req('GET', '/messages?to=' + encodeURIComponent(to));
  assert.equal(r.status, 200);
  assert.equal(r.json.messages.length, 1);

  r = await req('POST', '/admin/revoke', ADMIN, { token: TOKEN });
  assert.equal(r.status, 200);
  assert.equal(r.json.revoked, true);

  r = await req('POST', '/send', TOKEN, { to, subject: 'Again', body: 'Hello' });
  assert.equal(r.status, 401);

  r = await req('GET', '/messages?to=' + encodeURIComponent(to));
  assert.equal(r.json.messages.length, 1);
});

test('admin endpoints reject wrong token; send rejects missing token', async () => {
  let r = await req('POST', '/admin/reset', 'wrong-admin');
  assert.equal(r.status, 403);
  r = await req('POST', '/send', null, { to: 'a@example.test', subject: 'x', body: 'y' });
  assert.equal(r.status, 401);
});

test('reset clears messages and restores the token', async () => {
  let r = await req('POST', '/admin/reset', ADMIN);
  assert.equal(r.status, 200);
  r = await req('GET', '/messages');
  assert.equal(r.json.messages.length, 0);
  r = await req('POST', '/send', TOKEN, { to: 'b@example.test', subject: 'Hi', body: 'There' });
  assert.equal(r.status, 200);
});
