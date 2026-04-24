import { createBackendPlugin, coreServices } from '@backstage/backend-plugin-api';

import { createEngineProxyRouter } from './service/engineProxy';
import { CatalogSync } from './service/catalogSync';

type PluginLogger = {
  info(msg: string): void;
  warn?(msg: string): void;
  error(msg: string, err?: unknown): void;
};

type PluginConfig = {
  getString(key: string): string;
  getOptionalString(key: string): string | undefined;
};

/**
 * resolveEngineConfig reads the v0.2.0 keys (`rampart.engine.baseUrl`,
 * `rampart.engine.authToken`) and falls back to the v0.1.x
 * `rampart.baseUrl` so existing deployments keep working. A warn log
 * nudges operators to the new key on the fallback path.
 */
export function resolveEngineConfig(
  config: PluginConfig,
  logger: Pick<PluginLogger, 'info' | 'warn'>,
): { baseUrl: string; authToken?: string } {
  const explicit = config.getOptionalString('rampart.engine.baseUrl');
  const legacy = config.getOptionalString('rampart.baseUrl');
  let baseUrl = explicit;
  if (!baseUrl && legacy) {
    logger.warn?.(
      "rampart-backend: 'rampart.baseUrl' is deprecated — move the engine URL under 'rampart.engine.baseUrl'",
    );
    baseUrl = legacy;
  }
  if (!baseUrl) {
    throw new Error(
      "rampart-backend: no engine URL configured — set 'rampart.engine.baseUrl' in app-config",
    );
  }
  const authToken = config.getOptionalString('rampart.engine.authToken');
  return { baseUrl, authToken };
}

/**
 * rampart-backend registers /api/rampart/* on the Backstage backend.
 *
 * - /api/rampart/v1/*     → proxied to the engine (with service JWT
 *                           attached when `rampart.engine.authToken`
 *                           is configured)
 * - /api/rampart/_health  → backend-local readiness
 *
 * Also spins up a CatalogSync job that pushes Component entities to
 * the engine so the engine's matching has something to match against.
 */
export const rampartPlugin = createBackendPlugin({
  pluginId: 'rampart',
  register(env: any) {
    env.registerInit({
      deps: {
        logger: coreServices.logger,
        config: coreServices.rootConfig,
        httpRouter: coreServices.httpRouter,
      },
      async init({
        logger,
        config,
        httpRouter,
      }: {
        logger: PluginLogger;
        config: PluginConfig;
        httpRouter: { use(router: unknown): void };
      }) {
        const { baseUrl, authToken } = resolveEngineConfig(config, logger);
        const suffix = authToken ? ' (service JWT attached)' : '';
        logger.info(`rampart-backend: proxying /api/rampart/* to ${baseUrl}${suffix}`);

        const router = createEngineProxyRouter({ baseUrl, authToken, logger });
        httpRouter.use(router);

        const sync = new CatalogSync({ logger, config, baseUrl, authToken });
        sync.start();
      },
    });
  },
});
