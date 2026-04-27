import { RampartClient } from './client';

type DiscoveryApi = { getBaseUrl(pluginId: string): Promise<string> };
type FetchApi = { fetch: jest.Mock };

function mkDiscovery(base: string): DiscoveryApi {
  return { getBaseUrl: jest.fn().mockResolvedValue(base) };
}

function mkFetch(response: Partial<Response>): FetchApi {
  return { fetch: jest.fn().mockResolvedValue(response) };
}

describe('RampartClient', () => {
  it('routes every call through fetchApi so Backstage identity is attached', async () => {
    const discovery = mkDiscovery('http://backstage.test/api/rampart');
    const fetchApi = mkFetch({ ok: true, json: async () => ({ items: [] }) });

    const client = new RampartClient(discovery, fetchApi);
    await client.listIncidents();

    expect(discovery.getBaseUrl).toHaveBeenCalledWith('rampart');
    expect(fetchApi.fetch).toHaveBeenCalledWith(
      'http://backstage.test/api/rampart/v1/incidents',
    );
  });

  it('does not fall back to the global fetch', async () => {
    const globalFetch = jest.fn();
    const originalFetch = global.fetch;
    (global.fetch as unknown) = globalFetch;
    try {
      const discovery = mkDiscovery('http://backstage.test/api/rampart');
      const fetchApi = mkFetch({ ok: true, json: async () => ({ items: [] }) });
      const client = new RampartClient(discovery, fetchApi);
      await client.listIncidents();
      expect(globalFetch).not.toHaveBeenCalled();
    } finally {
      (global.fetch as unknown) = originalFetch;
    }
  });

  it('surfaces HTTP errors from getIncident as thrown errors', async () => {
    const discovery = mkDiscovery('http://backstage.test/api/rampart');
    const fetchApi = mkFetch({ ok: false, status: 404 });
    const client = new RampartClient(discovery, fetchApi);
    await expect(client.getIncident('inc-42')).rejects.toThrow(/getIncident 404/);
  });

  it('encodes the component ref in listSBOMsForComponent', async () => {
    const discovery = mkDiscovery('http://backstage.test/api/rampart');
    const fetchApi = mkFetch({ ok: true, json: async () => [] });
    const client = new RampartClient(discovery, fetchApi);
    await client.listSBOMsForComponent('kind:Component/default/web-app');
    expect(fetchApi.fetch).toHaveBeenCalledWith(
      'http://backstage.test/api/rampart/v1/components/kind%3AComponent%2Fdefault%2Fweb-app/sboms',
    );
  });

  it('hits the joined detail endpoint with an encoded id', async () => {
    const discovery = mkDiscovery('http://backstage.test/api/rampart');
    const fetchApi = mkFetch({
      ok: true,
      json: async () => ({
        incident: { id: 'inc-42', state: 'pending', iocId: 'ioc-1', openedAt: 't', lastTransitionedAt: 't' },
        affectedComponents: [],
      }),
    });
    const client = new RampartClient(discovery, fetchApi);
    const detail = await client.getIncidentDetail('inc-42');
    expect(fetchApi.fetch).toHaveBeenCalledWith(
      'http://backstage.test/api/rampart/v1/incidents/inc-42/detail',
    );
    expect(detail.incident.id).toBe('inc-42');
  });

  it('surfaces 404 from getIncidentDetail as a thrown error', async () => {
    const discovery = mkDiscovery('http://backstage.test/api/rampart');
    const fetchApi = mkFetch({ ok: false, status: 404 });
    const client = new RampartClient(discovery, fetchApi);
    await expect(client.getIncidentDetail('nope')).rejects.toThrow(/getIncidentDetail 404/);
  });
});
