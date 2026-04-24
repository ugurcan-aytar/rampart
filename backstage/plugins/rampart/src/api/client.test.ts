import { RampartClient } from './client';

type DiscoveryApi = { getBaseUrl(pluginId: string): Promise<string> };

function mkDiscovery(base: string): DiscoveryApi {
  return { getBaseUrl: jest.fn().mockResolvedValue(base) };
}

describe('RampartClient', () => {
  const originalFetch = global.fetch;
  afterEach(() => {
    (global.fetch as unknown) = originalFetch;
  });

  it('resolves URLs through discoveryApi instead of a hard-coded baseUrl', async () => {
    const discovery = mkDiscovery('http://backstage.test/api/rampart');
    const fetchMock = jest.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ items: [] }),
    });
    (global.fetch as unknown) = fetchMock;

    const client = new RampartClient(discovery);
    await client.listIncidents();

    expect(discovery.getBaseUrl).toHaveBeenCalledWith('rampart');
    expect(fetchMock).toHaveBeenCalledWith('http://backstage.test/api/rampart/v1/incidents');
  });

  it('surfaces HTTP errors from getIncident as thrown errors', async () => {
    const discovery = mkDiscovery('http://backstage.test/api/rampart');
    (global.fetch as unknown) = jest.fn().mockResolvedValue({
      ok: false,
      status: 404,
    });
    const client = new RampartClient(discovery);
    await expect(client.getIncident('inc-42')).rejects.toThrow(/getIncident 404/);
  });

  it('encodes the component ref in listSBOMsForComponent', async () => {
    const discovery = mkDiscovery('http://backstage.test/api/rampart');
    const fetchMock = jest.fn().mockResolvedValue({
      ok: true,
      json: async () => [],
    });
    (global.fetch as unknown) = fetchMock;
    const client = new RampartClient(discovery);
    await client.listSBOMsForComponent('kind:Component/default/web-app');
    expect(fetchMock).toHaveBeenCalledWith(
      'http://backstage.test/api/rampart/v1/components/kind%3AComponent%2Fdefault%2Fweb-app/sboms',
    );
  });
});
