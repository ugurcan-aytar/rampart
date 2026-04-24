import type { components } from './gen/schema';
import type { RampartApi, StreamEvent } from './ref';

type Incident = components['schemas']['Incident'];
type IncidentPage = components['schemas']['IncidentPage'];
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
 * RampartClient speaks the engine's HTTP + SSE contract. The base URL
 * is resolved via Backstage's discoveryApi — `rampart` maps to
 * `${backend.baseUrl}/api/rampart` — so the frontend never hits the
 * engine directly. Every fetch is same-origin against Backstage, which
 * removes the v0.1.x CORS handshake. Engine auth (JWT) is attached by
 * the rampart-backend proxy, not here.
 */
export class RampartClient implements RampartApi {
  private stream: EventSource | null = null;

  constructor(private readonly discovery: DiscoveryApi) {}

  async listIncidents(): Promise<Incident[]> {
    const base = await this.discovery.getBaseUrl('rampart');
    const res = await fetch(`${base}/v1/incidents`);
    if (!res.ok) {
      throw new Error(`rampart: listIncidents ${res.status}`);
    }
    const page = (await res.json()) as IncidentPage;
    return page.items;
  }

  async getIncident(id: string): Promise<Incident> {
    const base = await this.discovery.getBaseUrl('rampart');
    const res = await fetch(`${base}/v1/incidents/${encodeURIComponent(id)}`);
    if (!res.ok) {
      throw new Error(`rampart: getIncident ${res.status}`);
    }
    return (await res.json()) as Incident;
  }

  async listSBOMsForComponent(componentRef: string): Promise<SBOM[]> {
    const base = await this.discovery.getBaseUrl('rampart');
    const path = `${base}/v1/components/${encodeURIComponent(componentRef)}/sboms`;
    const res = await fetch(path);
    if (!res.ok) {
      throw new Error(`rampart: listSBOMsForComponent ${res.status}`);
    }
    return (await res.json()) as SBOM[];
  }

  subscribeToStream(handler: (event: StreamEvent) => void): () => void {
    // EventSource takes a URL synchronously. Resolve the base URL first,
    // then wire up the stream. If the unsubscribe is called before the
    // URL resolves, `cancelled` swallows the late connection.
    let cancelled = false;
    this.discovery
      .getBaseUrl('rampart')
      .then(base => {
        if (cancelled) return;
        this.stream = new EventSource(`${base}/v1/stream`);
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
