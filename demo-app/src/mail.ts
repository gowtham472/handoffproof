// Confirmation email goes through the HTTP mail provider (mail sink mock).
export async function sendMail(to: string, subject: string, body: string): Promise<void> {
  const base = process.env.MAIL_SINK_URL || 'http://localhost:4100';
  const token = process.env.MAIL_API_TOKEN || '';
  const res = await fetch(base + '/send', {
    signal: AbortSignal.timeout(5000),
    method: 'POST',
    headers: {
      'content-type': 'application/json',
      authorization: 'Bearer ' + token,
    },
    body: JSON.stringify({ to, subject, body }),
  });
  if (!res.ok) {
    throw new Error('mail provider returned ' + res.status);
  }
}
