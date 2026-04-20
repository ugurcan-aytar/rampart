/**
 * rampart plugin config schema. Declaring the frontend visibility here
 * is what tells Backstage's config-loader to ship `rampart.baseUrl` into
 * the browser bundle — otherwise it's stripped and `configApi.getString`
 * throws at render time.
 */
export interface Config {
  rampart: {
    /**
     * Base URL the rampart engine listens on. RampartClient prepends this
     * to /v1/... and /v1/stream calls.
     * @visibility frontend
     */
    baseUrl: string;
  };
}
