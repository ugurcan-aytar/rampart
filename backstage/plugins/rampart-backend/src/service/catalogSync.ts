import { createHash } from 'crypto';

type Logger = {
  info(msg: string): void;
  warn?(msg: string): void;
  error(msg: string, err?: unknown): void;
};
type Config = { getString(key: string): string; getOptionalString(key: string): string | undefined };

/**
 * CatalogEntity is the narrow slice of the Backstage Entity shape the
 * sync needs. Typed locally rather than imported from
 * `@backstage/catalog-model` so the test file can hand-roll fixtures
 * without pulling the full type graph.
 */
export type CatalogEntity = {
  kind: string;
  metadata: {
    name: string;
    namespace?: string;
    tags?: string[];
    annotations?: Record<string, string>;
  };
  spec?: {
    owner?: string;
    system?: string;
    lifecycle?: string;
    dependsOn?: string[];
  };
  relations?: { type: string; targetRef: string }[];
};

/**
 * CatalogApi is the one-method subset of `@backstage/catalog-client`'s
 * CatalogApi that the sync consumes. Narrow by design so test mocks
 * stay tiny and the real CatalogClient drops in without a wrapper.
 */
export type CatalogApi = {
  getEntities(
    request: { filter: { kind: string } },
  ): Promise<{ items: CatalogEntity[] }>;
};

/**
 * EngineComponent is the payload shape `/v1/components` accepts. Keep
 * in sync with `schemas/openapi.yaml` `Component`; duplicated here so
 * the plugin does not have to import the engine's generated types.
 */
export type EngineComponent = {
  ref: string;
  kind: string;
  namespace: string;
  name: string;
  owner?: string;
  system?: string;
  lifecycle?: string;
  tags?: string[];
  annotations?: Record<string, string>;
};

export type SyncStats = {
  pushed: number;
  skipped: number;
  failed: number;
  cycles: number;
};

/**
 * CatalogSync mirrors Backstage Component entities into the engine so
 * incident matching has a catalog to resolve blast radius against.
 *
 * Per tick the sync:
 *   1. Queries the Backstage catalog for Component entities.
 *   2. Resolves a push order via Kahn's algorithm on `spec.dependsOn`
 *      so dependencies land before dependents.
 *   3. Skips components whose content hash matches the previous tick
 *      (client-side memo — engine-side ETag is a v0.3.0+ concern).
 *   4. POSTs the remainder to `${engineBaseUrl}/v1/components`, one at
 *      a time, with the optional static service JWT attached.
 *   5. On a dependency cycle, warns and pushes the non-cyclic subset
 *      rather than blocking the whole sync.
 */
export class CatalogSync {
  private readonly logger: Logger;
  private readonly engineBaseUrl: string;
  private readonly authToken: string | undefined;
  private readonly intervalMs: number;
  private readonly catalog: CatalogApi | null;
  private readonly fetchImpl: typeof fetch;
  private readonly memo = new Map<string, string>();
  private timer: ReturnType<typeof setInterval> | null = null;

  constructor(opts: {
    logger: Logger;
    config: Config;
    baseUrl: string;
    authToken?: string;
    catalog?: CatalogApi;
    fetchImpl?: typeof fetch;
  }) {
    this.logger = opts.logger;
    this.engineBaseUrl = opts.baseUrl;
    this.authToken = opts.authToken;
    this.catalog = opts.catalog ?? null;
    this.fetchImpl = opts.fetchImpl ?? fetch;
    const freq = opts.config.getOptionalString('rampart.catalogSyncInterval');
    this.intervalMs = freq ? parseInterval(freq) : 24 * 60 * 60 * 1000;
  }

  start(): void {
    this.logger.info(
      `rampart CatalogSync: pushing Component entities to ${this.engineBaseUrl} every ${this.intervalMs} ms`,
    );
    this.timer = setInterval(() => {
      this.runOnce().catch(err => {
        this.logger.error('rampart CatalogSync failed', err);
      });
    }, this.intervalMs);
  }

  stop(): void {
    if (this.timer) {
      clearInterval(this.timer);
      this.timer = null;
    }
  }

  async runOnce(): Promise<SyncStats> {
    if (!this.catalog) {
      this.logger.info(
        'rampart CatalogSync: no catalog client configured — skipping tick',
      );
      return { pushed: 0, skipped: 0, failed: 0, cycles: 0 };
    }
    const { items } = await this.catalog.getEntities({
      filter: { kind: 'Component' },
    });
    const stats: SyncStats = { pushed: 0, skipped: 0, failed: 0, cycles: 0 };
    if (items.length === 0) return stats;

    const { order, cycles } = topoSortComponents(items, this.logger);
    stats.cycles = cycles;

    for (const entity of order) {
      const component = toEngineComponent(entity);
      if (!component) {
        stats.failed++;
        continue;
      }
      const hash = contentHash(component);
      if (this.memo.get(component.ref) === hash) {
        stats.skipped++;
        continue;
      }
      try {
        await this.pushOne(component);
        this.memo.set(component.ref, hash);
        stats.pushed++;
      } catch (err) {
        this.logger.error(
          `rampart CatalogSync: upsert failed for ${component.ref}`,
          err,
        );
        stats.failed++;
      }
    }
    this.logger.info(
      `rampart CatalogSync: tick done (pushed=${stats.pushed} skipped=${stats.skipped} failed=${stats.failed} cycles=${stats.cycles})`,
    );
    return stats;
  }

  private async pushOne(c: EngineComponent): Promise<void> {
    const headers = new Headers({ 'Content-Type': 'application/json' });
    if (this.authToken) {
      headers.set('Authorization', `Bearer ${this.authToken}`);
    }
    const res = await this.fetchImpl(`${this.engineBaseUrl}/v1/components`, {
      method: 'POST',
      headers,
      body: JSON.stringify(c),
    });
    if (!res.ok) {
      // Drop the hash on failure so the next tick retries; no exponential
      // backoff here — engine errors are the operator's signal.
      this.memo.delete(c.ref);
      const text = await res.text().catch(() => '');
      throw new Error(`engine ${res.status}: ${text.slice(0, 200)}`);
    }
  }
}

/** parseInterval takes "30m", "1h", "2d" → milliseconds. */
export function parseInterval(expr: string): number {
  const match = /^(\d+)(ms|s|m|h|d)$/.exec(expr);
  if (!match) {
    throw new Error(`invalid interval: ${expr}`);
  }
  const n = Number(match[1]);
  switch (match[2]) {
    case 'ms':
      return n;
    case 's':
      return n * 1000;
    case 'm':
      return n * 60_000;
    case 'h':
      return n * 3_600_000;
    case 'd':
      return n * 86_400_000;
    default:
      throw new Error(`unreachable: ${expr}`);
  }
}

export function toEngineComponent(entity: CatalogEntity): EngineComponent | null {
  if (entity.kind !== 'Component') return null;
  const name = entity.metadata.name;
  if (!name) return null;
  const namespace = entity.metadata.namespace ?? 'default';
  const kind = entity.kind;
  const ref = `kind:${kind}/${namespace}/${name}`;
  const out: EngineComponent = { ref, kind, namespace, name };
  const spec = entity.spec ?? {};
  if (spec.owner) out.owner = spec.owner;
  if (spec.system) out.system = spec.system;
  if (spec.lifecycle) out.lifecycle = spec.lifecycle;
  if (entity.metadata.tags?.length) out.tags = entity.metadata.tags;
  if (entity.metadata.annotations && Object.keys(entity.metadata.annotations).length > 0) {
    out.annotations = entity.metadata.annotations;
  }
  return out;
}

/**
 * topoSortComponents applies Kahn's algorithm over `spec.dependsOn` /
 * `relations[type=dependsOn]`, producing an order where each component
 * appears after all of its dependencies. On a cycle, the sort returns
 * the non-cyclic prefix it managed to produce; the rest is logged and
 * skipped — the engine never sees a broken graph.
 *
 * Unknown dependency targets (e.g. a `dependsOn` that points at a
 * component the catalog does not return) are treated as satisfied —
 * the sync is not the place to validate catalog integrity.
 */
export function topoSortComponents(
  entities: CatalogEntity[],
  logger?: Pick<Logger, 'warn'>,
): { order: CatalogEntity[]; cycles: number } {
  const byRef = new Map<string, CatalogEntity>();
  for (const e of entities) {
    byRef.set(refOf(e), e);
  }

  const deps = new Map<string, Set<string>>();
  const reverseDeps = new Map<string, Set<string>>();
  for (const e of entities) {
    const self = refOf(e);
    deps.set(self, new Set());
    reverseDeps.set(self, new Set());
  }
  for (const e of entities) {
    const self = refOf(e);
    for (const dep of dependenciesOf(e)) {
      if (!byRef.has(dep)) continue;
      deps.get(self)!.add(dep);
      reverseDeps.get(dep)!.add(self);
    }
  }

  const ready: string[] = [];
  for (const [ref, set] of deps) {
    if (set.size === 0) ready.push(ref);
  }
  ready.sort(); // stable order for tests

  const order: CatalogEntity[] = [];
  while (ready.length > 0) {
    const ref = ready.shift()!;
    order.push(byRef.get(ref)!);
    for (const dependent of reverseDeps.get(ref) ?? []) {
      const set = deps.get(dependent)!;
      set.delete(ref);
      if (set.size === 0) {
        ready.push(dependent);
        ready.sort();
      }
    }
  }

  const cyclic = entities.length - order.length;
  if (cyclic > 0) {
    const leftover = [...deps.entries()]
      .filter(([, set]) => set.size > 0)
      .map(([ref]) => ref);
    logger?.warn?.(
      `rampart CatalogSync: dependency cycle involving ${leftover.join(', ')} — skipping ${cyclic} component(s)`,
    );
  }
  return { order, cycles: cyclic };
}

function refOf(e: CatalogEntity): string {
  const kind = e.kind;
  const namespace = e.metadata.namespace ?? 'default';
  const name = e.metadata.name;
  return `${kind.toLowerCase()}:${namespace}/${name}`;
}

function dependenciesOf(e: CatalogEntity): string[] {
  const out = new Set<string>();
  for (const t of e.spec?.dependsOn ?? []) {
    out.add(normaliseDep(t));
  }
  for (const r of e.relations ?? []) {
    if (r.type === 'dependsOn') out.add(normaliseDep(r.targetRef));
  }
  return [...out];
}

function normaliseDep(target: string): string {
  // Backstage refs come in two shapes: `component:default/auth` (full)
  // and `auth` (short, same kind + default namespace). Anything without
  // a colon is assumed to be a `component:default/` short form — that
  // matches how the Backstage catalog itself resolves `spec.dependsOn`.
  if (target.includes(':')) return target.toLowerCase();
  return `component:default/${target.toLowerCase()}`;
}

function contentHash(c: EngineComponent): string {
  // Stable key ordering so a Map re-shuffle doesn't look like a change.
  const ordered = {
    ref: c.ref,
    kind: c.kind,
    namespace: c.namespace,
    name: c.name,
    owner: c.owner ?? null,
    system: c.system ?? null,
    lifecycle: c.lifecycle ?? null,
    tags: c.tags ?? null,
    annotations: c.annotations ?? null,
  };
  return createHash('sha256').update(JSON.stringify(ordered)).digest('hex');
}
