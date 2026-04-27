import { createApiRef } from '@backstage/core-plugin-api';

import type { components } from './gen/schema';

type Incident = components['schemas']['Incident'];
type IncidentDetail = components['schemas']['IncidentDetail'];
type PublisherSnapshot = components['schemas']['PublisherSnapshot'];
type SBOM = components['schemas']['SBOM'];

export type StreamEvent =
  | { type: 'incident.opened'; data: components['schemas']['IncidentOpenedEvent'] }
  | { type: 'incident.transitioned'; data: components['schemas']['IncidentTransitionedEvent'] }
  | { type: 'remediation.added'; data: components['schemas']['RemediationAddedEvent'] }
  | { type: 'sbom.ingested'; data: components['schemas']['SBOMIngestedEvent'] }
  | { type: 'ioc.matched'; data: components['schemas']['IoCMatchedEvent'] };

/**
 * IncidentListFilter mirrors the engine's ListIncidents query params
 * (Theme E3 backend, PR #40 commit d6ea9d6). Empty fields = no filter
 * on that dimension. Multi-value filters (states, ecosystems) are OR'd
 * within the dimension; all dimensions are AND'd together.
 */
export type IncidentListFilter = {
  states?: string[];
  ecosystems?: string[];
  from?: string;
  to?: string;
  search?: string;
  owner?: string;
  limit?: number;
};

export type RampartApi = {
  listIncidents(filter?: IncidentListFilter): Promise<Incident[]>;
  getIncident(id: string): Promise<Incident>;
  /**
   * getIncidentDetail returns the joined view used by IncidentDetailDrawer:
   * incident + matched IoC + every affected component hydrated. Single
   * round-trip — the engine resolves the joins in one handler call so the
   * drawer-open path stays under the 200ms budget.
   */
  getIncidentDetail(id: string): Promise<IncidentDetail>;
  /**
   * getPublisherHistory returns the snapshot time-series for a single
   * package, newest-first. Theme F1 ingests these from npm + GitHub;
   * Theme F3 PublisherAnomalyPanel reads them to render maintainer /
   * cadence / OIDC charts.
   */
  getPublisherHistory(packageRef: string, limit?: number): Promise<PublisherSnapshot[]>;
  listSBOMsForComponent(componentRef: string): Promise<SBOM[]>;
  subscribeToStream(handler: (event: StreamEvent) => void): () => void;
};

export const rampartApiRef = createApiRef<RampartApi>({
  id: 'plugin.rampart.api',
});
