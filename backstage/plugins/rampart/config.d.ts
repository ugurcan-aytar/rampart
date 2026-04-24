/**
 * rampart plugin frontend config schema.
 *
 * v0.1.x exported `rampart.baseUrl` here so the browser bundle could
 * learn the engine origin. v0.2.0 routes all frontend traffic through
 * the Backstage backend proxy (`${backend.baseUrl}/api/rampart`) via
 * `discoveryApiRef`, so no rampart-specific frontend config is
 * required anymore. The file stays as an augmentation point — it is
 * how Backstage's config-loader recognises that the plugin has opted
 * into the frontend-visibility contract.
 */
export interface Config {}
