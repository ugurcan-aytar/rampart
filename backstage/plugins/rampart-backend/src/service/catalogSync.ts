type Logger = { info(msg: string): void; error(msg: string, err?: unknown): void };
type Config = { getString(key: string): string; getOptionalString(key: string): string | undefined };

/**
 * CatalogSync mirrors Backstage Component entities into the engine so
 * incident matching has a catalog to resolve blast radius against.
 *
 * Phase 1 — skeleton only: constructor wires deps, start() logs a
 * placeholder. The real sync (Adım 7 continuation) queries
 * `@backstage/catalog-client`, batches Component entities, and POSTs
 * them to /v1/components. A cron schedule (`setInterval` or the
 * scheduler service) triggers the sync nightly.
 */
export class CatalogSync {
  private readonly logger: Logger;
  private readonly engineBaseUrl: string;
  private readonly intervalMs: number;
  private timer: ReturnType<typeof setInterval> | null = null;

  constructor(opts: { logger: Logger; config: Config }) {
    this.logger = opts.logger;
    this.engineBaseUrl = opts.config.getString('rampart.baseUrl');
    const freq = opts.config.getOptionalString('rampart.catalogSyncInterval');
    this.intervalMs = freq ? parseInterval(freq) : 24 * 60 * 60 * 1000;
  }

  start(): void {
    this.logger.info(
      `rampart CatalogSync: will push Component entities to ${this.engineBaseUrl} every ${this.intervalMs} ms`,
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

  private async runOnce(): Promise<void> {
    this.logger.info('rampart CatalogSync: runOnce — not yet implemented (Adım 7 continuation)');
    // Adım 7:
    //   1. catalog.getEntities({ filter: { kind: 'Component' } })
    //   2. POST each to `${engineBaseUrl}/v1/components`
    //   3. record sync stats
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
