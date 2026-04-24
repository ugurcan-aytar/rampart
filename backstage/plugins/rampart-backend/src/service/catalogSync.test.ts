import {
  CatalogSync,
  topoSortComponents,
  toEngineComponent,
  type CatalogApi,
  type CatalogEntity,
  type EngineComponent,
} from './catalogSync';

function mkLogger() {
  return { info: jest.fn(), warn: jest.fn(), error: jest.fn() };
}

function mkConfig(values: Record<string, string> = {}) {
  return {
    getString(key: string): string {
      const v = values[key];
      if (v === undefined) throw new Error(`missing config: ${key}`);
      return v;
    },
    getOptionalString(key: string): string | undefined {
      return values[key];
    },
  };
}

function mkCatalog(items: CatalogEntity[]): CatalogApi {
  return { getEntities: jest.fn().mockResolvedValue({ items }) };
}

function mkComponent(name: string, deps: string[] = [], namespace = 'default'): CatalogEntity {
  return {
    kind: 'Component',
    metadata: { name, namespace, tags: ['demo'] },
    spec: { owner: 'team:platform', lifecycle: 'production', dependsOn: deps },
  };
}

function fetchOk(): jest.Mock {
  return jest.fn().mockResolvedValue({
    ok: true,
    status: 200,
    text: async () => '{}',
  });
}

function mkSync(opts: {
  catalog?: CatalogApi;
  fetchImpl?: typeof fetch;
  authToken?: string;
} = {}) {
  const logger = mkLogger();
  const sync = new CatalogSync({
    logger,
    config: mkConfig({ 'rampart.catalogSyncInterval': '30m' }),
    baseUrl: 'http://engine.test:8080',
    authToken: opts.authToken,
    catalog: opts.catalog,
    fetchImpl: opts.fetchImpl,
  });
  return { sync, logger };
}

describe('toEngineComponent', () => {
  it('maps a Backstage Component onto the engine shape', () => {
    const entity = mkComponent('web-app', ['component:default/auth']);
    entity.metadata.annotations = { 'rampart.io/scope': 'core' };
    const c = toEngineComponent(entity)!;
    expect(c.ref).toBe('kind:Component/default/web-app');
    expect(c.owner).toBe('team:platform');
    expect(c.lifecycle).toBe('production');
    expect(c.tags).toEqual(['demo']);
    expect(c.annotations).toEqual({ 'rampart.io/scope': 'core' });
  });

  it('rejects non-Component entities', () => {
    expect(toEngineComponent({ kind: 'API', metadata: { name: 'a' } })).toBeNull();
  });

  it('defaults namespace to `default` when unset', () => {
    const c = toEngineComponent({ kind: 'Component', metadata: { name: 'x' } })!;
    expect(c.namespace).toBe('default');
    expect(c.ref).toBe('kind:Component/default/x');
  });
});

describe('topoSortComponents', () => {
  it('is a no-op on an empty catalog', () => {
    expect(topoSortComponents([])).toEqual({ order: [], cycles: 0 });
  });

  it('orders linear dependencies so deps land first', () => {
    const a = mkComponent('a');
    const b = mkComponent('b', ['a']);
    const c = mkComponent('c', ['b']);
    const { order, cycles } = topoSortComponents([c, a, b]);
    expect(order.map(e => e.metadata.name)).toEqual(['a', 'b', 'c']);
    expect(cycles).toBe(0);
  });

  it('honours relations[type=dependsOn] alongside spec.dependsOn', () => {
    const a = mkComponent('a');
    const b: CatalogEntity = {
      kind: 'Component',
      metadata: { name: 'b', namespace: 'default' },
      relations: [{ type: 'dependsOn', targetRef: 'component:default/a' }],
    };
    const { order } = topoSortComponents([b, a]);
    expect(order.map(e => e.metadata.name)).toEqual(['a', 'b']);
  });

  it('warns and partially orders when a cycle exists', () => {
    const logger = { warn: jest.fn() };
    const a = mkComponent('a', ['b']);
    const b = mkComponent('b', ['a']);
    const c = mkComponent('c');
    const { order, cycles } = topoSortComponents([a, b, c], logger);
    expect(order.map(e => e.metadata.name)).toEqual(['c']);
    expect(cycles).toBe(2);
    expect(logger.warn).toHaveBeenCalledWith(expect.stringContaining('cycle'));
  });

  it('ignores dependencies pointing at components outside the catalog', () => {
    const a = mkComponent('a', ['component:default/not-in-catalog']);
    const { order, cycles } = topoSortComponents([a]);
    expect(order.map(e => e.metadata.name)).toEqual(['a']);
    expect(cycles).toBe(0);
  });
});

describe('CatalogSync.runOnce', () => {
  it('returns zero stats when the catalog has no components', async () => {
    const { sync } = mkSync({ catalog: mkCatalog([]) });
    const stats = await sync.runOnce();
    expect(stats).toEqual({ pushed: 0, skipped: 0, failed: 0, cycles: 0 });
  });

  it('pushes each component in dependency order', async () => {
    const fetchMock = fetchOk();
    const { sync } = mkSync({
      catalog: mkCatalog([mkComponent('b', ['a']), mkComponent('a')]),
      fetchImpl: fetchMock as unknown as typeof fetch,
    });
    const stats = await sync.runOnce();
    expect(stats.pushed).toBe(2);
    expect(stats.skipped).toBe(0);
    const firstRefBody = JSON.parse(fetchMock.mock.calls[0][1].body as string) as EngineComponent;
    const secondRefBody = JSON.parse(fetchMock.mock.calls[1][1].body as string) as EngineComponent;
    expect(firstRefBody.name).toBe('a');
    expect(secondRefBody.name).toBe('b');
  });

  it('attaches Authorization when authToken is set', async () => {
    const fetchMock = fetchOk();
    const { sync } = mkSync({
      catalog: mkCatalog([mkComponent('a')]),
      fetchImpl: fetchMock as unknown as typeof fetch,
      authToken: 'svc-token',
    });
    await sync.runOnce();
    const init = fetchMock.mock.calls[0][1] as { headers: Headers };
    expect(init.headers.get('Authorization')).toBe('Bearer svc-token');
  });

  it('memoises content and skips unchanged components on the next tick', async () => {
    const fetchMock = fetchOk();
    const { sync } = mkSync({
      catalog: mkCatalog([mkComponent('a')]),
      fetchImpl: fetchMock as unknown as typeof fetch,
    });
    const first = await sync.runOnce();
    expect(first.pushed).toBe(1);
    const second = await sync.runOnce();
    expect(second.pushed).toBe(0);
    expect(second.skipped).toBe(1);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('re-pushes a component when any field changes', async () => {
    const fetchMock = fetchOk();
    const first = mkComponent('a');
    const second = mkComponent('a');
    second.spec!.lifecycle = 'deprecated';

    const catalog: CatalogApi = { getEntities: jest.fn() };
    (catalog.getEntities as jest.Mock)
      .mockResolvedValueOnce({ items: [first] })
      .mockResolvedValueOnce({ items: [second] });

    const { sync } = mkSync({ catalog, fetchImpl: fetchMock as unknown as typeof fetch });
    await sync.runOnce();
    const stats = await sync.runOnce();
    expect(stats.pushed).toBe(1);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('counts engine failures and clears the memo so the next tick retries', async () => {
    const fetchMock = jest
      .fn()
      .mockResolvedValueOnce({ ok: false, status: 500, text: async () => 'boom' })
      .mockResolvedValueOnce({ ok: true, status: 200, text: async () => '{}' });
    const { sync, logger } = mkSync({
      catalog: mkCatalog([mkComponent('a')]),
      fetchImpl: fetchMock as unknown as typeof fetch,
    });
    const first = await sync.runOnce();
    expect(first.failed).toBe(1);
    expect(first.pushed).toBe(0);
    expect(logger.error).toHaveBeenCalled();
    const second = await sync.runOnce();
    expect(second.pushed).toBe(1);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
