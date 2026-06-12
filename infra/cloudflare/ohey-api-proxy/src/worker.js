const ORIGIN = 'https://ohey-backend.onrender.com';

export default {
  async fetch(request) {
    const incomingUrl = new URL(request.url);
    const originUrl = new URL(incomingUrl.pathname + incomingUrl.search, ORIGIN);

    const headers = new Headers(request.headers);
    headers.set('Host', new URL(ORIGIN).host);
    headers.set('X-Forwarded-Host', incomingUrl.host);
    headers.set('X-Forwarded-Proto', incomingUrl.protocol.replace(':', ''));

    return fetch(originUrl.toString(), {
      method: request.method,
      headers,
      body: request.body,
      redirect: 'manual',
    });
  },
};
