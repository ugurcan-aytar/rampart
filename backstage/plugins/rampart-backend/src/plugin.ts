import { createBackendPlugin, coreServices } from '@backstage/backend-plugin-api';

import { createEngineProxyRouter } from './service/engineProxy';
import { CatalogSync } from './service/catalogSync';

/**
 * rampart-backend registers /api/rampart/* on the Backstage backend.
 *
 * - /api/rampart/v1/*     → proxied to the engine
 * - /api/rampart/_health  → backend-local readiness
 *
 * Uses the modern Backstage backend system (createBackendPlugin,
 * registerInit) rather than the legacy createRouter. Also spins up a
 * CatalogSync job that pushes Component entities to the engine so the
 * engine's matching has something to match against.
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
        logger: { info(msg: string): void; error(msg: string, err?: unknown): void };
        config: { getString(key: string): string; getOptionalString(key: string): string | undefined };
        httpRouter: { use(router: unknown): void };
      }) {
        const baseUrl = config.getString('rampart.baseUrl');
        logger.info(`rampart-backend: proxying /api/rampart/* to ${baseUrl}`);

        const router = createEngineProxyRouter({ baseUrl, logger });
        httpRouter.use(router);

        const sync = new CatalogSync({ logger, config });
        sync.start();
      },
    });
  },
});
