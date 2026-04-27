import { createApiRef } from '@backstage/core-plugin-api';

import type { components } from './gen/schema';

type Incident = components['schemas']['Incident'];
type IncidentDetail = components['schemas']['IncidentDetail'];
type SBOM = components['schemas']['SBOM'];

export type StreamEvent =
  | { type: 'incident.opened'; data: components['schemas']['IncidentOpenedEvent'] }
  | { type: 'incident.transitioned'; data: components['schemas']['IncidentTransitionedEvent'] }
  | { type: 'remediation.added'; data: components['schemas']['RemediationAddedEvent'] }
  | { type: 'sbom.ingested'; data: components['schemas']['SBOMIngestedEvent'] }
  | { type: 'ioc.matched'; data: components['schemas']['IoCMatchedEvent'] };

export type RampartApi = {
  listIncidents(): Promise<Incident[]>;
  getIncident(id: string): Promise<Incident>;
  /**
   * getIncidentDetail returns the joined view used by IncidentDetailDrawer:
   * incident + matched IoC + every affected component hydrated. Single
   * round-trip — the engine resolves the joins in one handler call so the
   * drawer-open path stays under the 200ms budget.
   */
  getIncidentDetail(id: string): Promise<IncidentDetail>;
  listSBOMsForComponent(componentRef: string): Promise<SBOM[]>;
  subscribeToStream(handler: (event: StreamEvent) => void): () => void;
};

export const rampartApiRef = createApiRef<RampartApi>({
  id: 'plugin.rampart.api',
});
