import express from 'express';
import type { Request, Response } from 'express';
import { createServer } from 'http';
import type { Server } from 'http';
import type { AddressInfo } from 'net';

import { createEngineProxyRouter } from './engineProxy';

type CapturedRequest = {
  method: string;
  path: string;
  authorization: string | undefined;
  contentType: string | undefined;
  body: string;
};

function mkLogger() {
  return { info: jest.fn(), error: jest.fn() };
}

async function startUpstream(): Promise<{
  url: string;
  server: Server;
  lastRequest: () => CapturedRequest | null;
}> {
  let captured: CapturedRequest | null = null;
  const upstream = express();
  upstream.use(express.text({ type: '*/*' }));
  upstream.all(/.*/, (req: Request, res: Response) => {
    captured = {
      method: req.method,
      path: req.path,
      authorization: req.header('authorization') ?? undefined,
      contentType: req.header('content-type') ?? undefined,
      body: typeof req.body === 'string' ? req.body : '',
    };
    res.status(200).json({ ok: true });
  });
  const server = createServer(upstream);
  await new Promise<void>(resolve => server.listen(0, resolve));
  const { port } = server.address() as AddressInfo;
  return {
    url: `http://127.0.0.1:${port}`,
    server,
    lastRequest: () => captured,
  };
}

function startProxy(opts: { baseUrl: string; authToken?: string }): Promise<{
  url: string;
  server: Server;
}> {
  const app = express();
  app.use(express.json());
  const router = createEngineProxyRouter({
    baseUrl: opts.baseUrl,
    authToken: opts.authToken,
    logger: mkLogger(),
  });
  app.use(router);
  return new Promise(resolve => {
    const server = createServer(app);
    server.listen(0, () => {
      const { port } = server.address() as AddressInfo;
      resolve({ url: `http://127.0.0.1:${port}`, server });
    });
  });
}

async function stop(server: Server): Promise<void> {
  await new Promise<void>(resolve => server.close(() => resolve()));
}

describe('engineProxy', () => {
  it('attaches the configured bearer token when the caller sends none', async () => {
    const upstream = await startUpstream();
    const proxy = await startProxy({ baseUrl: upstream.url, authToken: 'svc-xyz' });
    try {
      const res = await fetch(`${proxy.url}/v1/components`);
      expect(res.status).toBe(200);
      const req = upstream.lastRequest()!;
      expect(req.path).toBe('/v1/components');
      expect(req.authorization).toBe('Bearer svc-xyz');
    } finally {
      await stop(proxy.server);
      await stop(upstream.server);
    }
  });

  it('forwards the caller-supplied Authorization header unchanged', async () => {
    const upstream = await startUpstream();
    const proxy = await startProxy({ baseUrl: upstream.url, authToken: 'svc-xyz' });
    try {
      const res = await fetch(`${proxy.url}/v1/components`, {
        headers: { Authorization: 'Bearer user-abc' },
      });
      expect(res.status).toBe(200);
      const req = upstream.lastRequest()!;
      expect(req.authorization).toBe('Bearer user-abc');
    } finally {
      await stop(proxy.server);
      await stop(upstream.server);
    }
  });

  it('omits Authorization when neither caller nor config supplies one', async () => {
    const upstream = await startUpstream();
    const proxy = await startProxy({ baseUrl: upstream.url });
    try {
      const res = await fetch(`${proxy.url}/v1/components`);
      expect(res.status).toBe(200);
      const req = upstream.lastRequest()!;
      expect(req.authorization).toBeUndefined();
    } finally {
      await stop(proxy.server);
      await stop(upstream.server);
    }
  });

  it('serves a backend-local health endpoint without hitting upstream', async () => {
    const upstream = await startUpstream();
    const proxy = await startProxy({ baseUrl: upstream.url });
    try {
      const res = await fetch(`${proxy.url}/_health`);
      expect(res.status).toBe(200);
      const json = await res.json();
      expect(json).toEqual({ status: 'ok' });
      expect(upstream.lastRequest()).toBeNull();
    } finally {
      await stop(proxy.server);
      await stop(upstream.server);
    }
  });
});
