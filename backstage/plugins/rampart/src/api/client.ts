import type { components } from './gen/schema';
import type { IncidentListFilter, RampartApi, StreamEvent } from './ref';

type Incident = components['schemas']['Incident'];
type IncidentDetail = components['schemas']['IncidentDetail'];
type IncidentPage = components['schemas']['IncidentPage'];
type PublisherHistory = components['schemas']['PublisherHistory'];
type PublisherSnapshot = components['schemas']['PublisherSnapshot'];
type SBOM = components['schemas']['SBOM'];

const STREAM_EVENT_TYPES = [
  'incident.opened',
  'incident.transitioned',
  'remediation.added',
  'sbom.ingested',
  'ioc.matched',
] as const;

/**
 * DiscoveryApi is the subset of Backstage's `discoveryApiRef` the client
 * uses. Typed locally so this file does not have to pull in the full
 * core-plugin-api type graph; the Backstage DiscoveryApi is a superset.
 */
type DiscoveryApi = { getBaseUrl(pluginId: string): Promise<string> };

/**
 * FetchApi is the subset of Backstage's `fetchApiRef`. Using it (rather
 * than the global `fetch`) is what attaches the current user's
 * Backstage identity token to every call — Backstage's `httpRouter`
 * rejects unauthenticated requests with 401, so direct `fetch()` calls
 * would never reach the rampart-backend proxy.
 */
type FetchApi = { fetch: typeof fetch };

/**
 * filterToQueryString serialises an IncidentListFilter into the
 * exact form ListIncidents handler expects. Multi-value filters
 * (states, ecosystems) emit one query parameter per value (matches
 * `style: form, explode: true` in the OpenAPI spec).
 */
function filterToQueryString(filter?: IncidentListFilter): string {
  if (!filter) return '';
  const params = new URLSearchParams();
  for (const s of filter.states ?? []) {
    if (s) params.append('state', s);
  }
  for (const e of filter.ecosystems ?? []) {
    if (e) params.append('ecosystem', e);
  }
  if (filter.from) params.set('from', filter.from);
  if (filter.to) params.set('to', filter.to);
  if (filter.search) params.set('search', filter.search);
  if (filter.owner) params.set('owner', filter.owner);
  if (filter.limit !== undefined) params.set('limit', String(filter.limit));
  return params.toString();
}

/**
 * RampartClient speaks the engine's HTTP + SSE contract. The base URL
 * is resolved via Backstage's discoveryApi — `rampart` maps to
 * `${backend.baseUrl}/api/rampart` — so the frontend never hits the
 * engine directly. Every fetch goes through Backstage's fetchApi so
 * the Backstage identity travels with the request; the rampart-backend
 * proxy then attaches the engine service JWT before forwarding.
 */
export class RampartClient implements RampartApi {
  private stream: EventSource | null = null;

  constructor(
    private readonly discovery: DiscoveryApi,
    private readonly fetchApi: FetchApi,
  ) {}

  async listIncidents(filter?: IncidentListFilter): Promise<Incident[]> {
    const base = await this.discovery.getBaseUrl('rampart');
    const qs = filterToQueryString(filter);
    const url = qs ? `${base}/v1/incidents?${qs}` : `${base}/v1/incidents`;
    const res = await this.fetchApi.fetch(url);
    if (!res.ok) {
      throw new Error(`rampart: listIncidents ${res.status}`);
    }
    const page = (await res.json()) as IncidentPage;
    return page.items;
  }

  async getIncident(id: string): Promise<Incident> {
    const base = await this.discovery.getBaseUrl('rampart');
    const res = await this.fetchApi.fetch(
      `${base}/v1/incidents/${encodeURIComponent(id)}`,
    );
    if (!res.ok) {
      throw new Error(`rampart: getIncident ${res.status}`);
    }
    return (await res.json()) as Incident;
  }

  async getIncidentDetail(id: string): Promise<IncidentDetail> {
    const base = await this.discovery.getBaseUrl('rampart');
    const res = await this.fetchApi.fetch(
      `${base}/v1/incidents/${encodeURIComponent(id)}/detail`,
    );
    if (!res.ok) {
      throw new Error(`rampart: getIncidentDetail ${res.status}`);
    }
    return (await res.json()) as IncidentDetail;
  }

  async getPublisherHistory(packageRef: string, limit?: number): Promise<PublisherSnapshot[]> {
    const base = await this.discovery.getBaseUrl('rampart');
    const qs = limit !== undefined ? `?limit=${limit}` : '';
    const path = `${base}/v1/publisher/${encodeURIComponent(packageRef)}/history${qs}`;
    const res = await this.fetchApi.fetch(path);
    if (!res.ok) {
      throw new Error(`rampart: getPublisherHistory ${res.status}`);
    }
    const page = (await res.json()) as PublisherHistory;
    return page.items;
  }

  async listSBOMsForComponent(componentRef: string): Promise<SBOM[]> {
    const base = await this.discovery.getBaseUrl('rampart');
    const path = `${base}/v1/components/${encodeURIComponent(componentRef)}/sboms`;
    const res = await this.fetchApi.fetch(path);
    if (!res.ok) {
      throw new Error(`rampart: listSBOMsForComponent ${res.status}`);
    }
    return (await res.json()) as SBOM[];
  }

  subscribeToStream(handler: (event: StreamEvent) => void): () => void {
    // EventSource takes a URL synchronously and has no way to attach
    // arbitrary headers (the WHATWG spec only allows `withCredentials`
    // for cookie-based auth). `withCredentials: true` forwards the
    // Backstage session cookie so the backend auth middleware accepts
    // the stream; this works for cookie-session Backstage deployments.
    // Token-only deployments that want SSE will need an out-of-band
    // mint mechanism — tracked alongside Theme A3 (Backstage OAuth).
    let cancelled = false;
    this.discovery
      .getBaseUrl('rampart')
      .then(base => {
        if (cancelled) return;
        this.stream = new EventSource(`${base}/v1/stream`, { withCredentials: true });
        STREAM_EVENT_TYPES.forEach(type => {
          this.stream!.addEventListener(type, (e: MessageEvent) => {
            try {
              const data = JSON.parse(e.data);
              handler({ type, data } as StreamEvent);
            } catch {
              // Malformed event; ignore.
            }
          });
        });
      })
      .catch(() => {
        // Discovery failure is transient; the dashboard renders an error
        // state from listIncidents/getIncident — no need to shout here.
      });

    return () => {
      cancelled = true;
      this.stream?.close();
      this.stream = null;
    };
  }
}
