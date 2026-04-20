import express from 'express';

type Logger = {
  info(msg: string): void;
  error(msg: string, err?: unknown): void;
};

type Req = { method: string; path: string; headers: Record<string, unknown>; body: unknown };
type Res = {
  status(code: number): Res;
  setHeader(name: string, value: string): void;
  json(body: unknown): void;
  send(body: string): void;
};

/**
 * createEngineProxyRouter returns an Express router mounted at the
 * rampart backend's base path (/api/rampart). Phase 1 is a thin
 * forwarder — it relays method / path / body to the engine and streams
 * the response back. Adım 7 adds auth forwarding, connection pooling,
 * and SSE passthrough for /v1/stream.
 */
export function createEngineProxyRouter(opts: { baseUrl: string; logger: Logger }) {
  const router = express();

  router.get('/_health', (_req: Req, res: Res) => {
    res.json({ status: 'ok' });
  });

  router.all('/v1/*', async (req: Req, res: Res) => {
    const target = opts.baseUrl + req.path;
    try {
      const upstream = await fetch(target, {
        method: req.method,
        headers: pickForwardHeaders(req.headers),
        body: ['GET', 'HEAD'].includes(req.method) ? undefined : JSON.stringify(req.body),
      });
      res.status(upstream.status);
      upstream.headers.forEach((value: string, key: string) => {
        // Hop-by-hop headers should not be forwarded — Adım 7 filters.
        res.setHeader(key, value);
      });
      const body = await upstream.text();
      res.send(body);
    } catch (err) {
      opts.logger.error(`rampart proxy failed to ${target}`, err);
      res.status(502).json({ code: 'PROXY_FAILED', message: String(err) });
    }
  });

  return router;
}

function pickForwardHeaders(headers: Record<string, unknown>): Record<string, string> {
  const out: Record<string, string> = {};
  const ct = headers['content-type'];
  if (typeof ct === 'string') out['Content-Type'] = ct;
  const auth = headers['authorization'];
  if (typeof auth === 'string') out['Authorization'] = auth;
  return out;
}
