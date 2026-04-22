import express from 'express';
import type { Request, Response, Express } from 'express';
import type { IncomingHttpHeaders } from 'http';

type Logger = {
  info(msg: string): void;
  error(msg: string, err?: unknown): void;
};

/**
 * createEngineProxyRouter returns an Express router mounted at the
 * rampart backend's base path (/api/rampart). Phase 1 is a thin
 * forwarder — it relays method / path / body to the engine and streams
 * the response back. Adım 7 adds auth forwarding, connection pooling,
 * and SSE passthrough for /v1/stream.
 *
 * Explicit Express return type keeps TS2742 ("inferred type cannot be
 * named portably") away when another module imports this function.
 */
export function createEngineProxyRouter(opts: { baseUrl: string; logger: Logger }): Express {
  const router = express();

  router.get('/_health', (_req: Request, res: Response) => {
    res.json({ status: 'ok' });
  });

  router.all('/v1/*splat', async (req: Request, res: Response) => {
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

function pickForwardHeaders(headers: IncomingHttpHeaders): Headers {
  // Using Headers (instead of a plain object) sidesteps eslint's
  // dot-notation rule complaining about literal header names like
  // 'Content-Type' and 'Authorization' while keeping the semantics
  // fetch() expects. Typing `headers` as IncomingHttpHeaders (named
  // fields, not an index signature) lets us dot-access authorization
  // and bracket-access 'content-type' — both satisfy TS4111 +
  // dot-notation simultaneously.
  const out = new Headers();
  const ct = headers['content-type'];
  if (typeof ct === 'string') out.set('Content-Type', ct);
  const auth = headers.authorization;
  if (typeof auth === 'string') out.set('Authorization', auth);
  return out;
}
