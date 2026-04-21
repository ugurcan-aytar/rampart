/*
 * Backstage backend entry point for the rampart example app.
 *
 * Intentionally thin: createBackend() loads the default feature set
 * (config, logger, auth, database, discovery, http-router, permissions),
 * then we attach the rampart-backend plugin which mounts
 * /api/rampart/v1/* as a proxy to the engine and starts the
 * CatalogSync tick loop.
 */
import { createBackend } from '@backstage/backend-defaults';
import { rampartPlugin } from '@ugurcan-aytar/backstage-plugin-rampart-backend';

const backend = createBackend();

// Serve the built frontend bundle (packages/app/dist) at the backend's
// root URL. Without this the backend happily serves /api/* but any /
// request (the Backstage SPA entrypoint) 404s.
backend.add(import('@backstage/plugin-app-backend'));

// The rampart-backend plugin exports `rampartPlugin` as a named export
// (not default), so we pass it directly rather than using the
// `backend.add(import(...))` dynamic-import shorthand — that shorthand
// expects a default export that IS a BackendFeature.
backend.add(rampartPlugin);

backend.start();
