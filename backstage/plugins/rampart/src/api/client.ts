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
 * RampartClient speaks the engine's HTTP + SSE contract. It uses the
 * native fetch and EventSource APIs (no axios, no sse-polyfill —
 * supported by every browser Backstage targets).
 */
export class RampartClient implements RampartApi {
  private stream: EventSource | null = null;

  constructor(private readonly baseUrl: string) {}

  async listIncidents(): Promise<Incident[]> {
    const res = await fetch(`${this.baseUrl}/v1/incidents`);
    if (!res.ok) {
      throw new Error(`rampart: listIncidents ${res.status}`);
    }
    const page = (await res.json()) as IncidentPage;
    return page.items;
  }

  async getIncident(id: string): Promise<Incident> {
    const res = await fetch(`${this.baseUrl}/v1/incidents/${encodeURIComponent(id)}`);
    if (!res.ok) {
      throw new Error(`rampart: getIncident ${res.status}`);
    }
    return (await res.json()) as Incident;
  }

  async listSBOMsForComponent(componentRef: string): Promise<SBOM[]> {
    const path = `${this.baseUrl}/v1/components/${encodeURIComponent(componentRef)}/sboms`;
    const res = await fetch(path);
    if (!res.ok) {
      throw new Error(`rampart: listSBOMsForComponent ${res.status}`);
    }
    return (await res.json()) as SBOM[];
  }

  subscribeToStream(handler: (event: StreamEvent) => void): () => void {
    this.stream = new EventSource(`${this.baseUrl}/v1/stream`);
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
    return () => {
      this.stream?.close();
      this.stream = null;
    };
  }
}
