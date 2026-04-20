import { createApiRef } from '@backstage/core-plugin-api';

import type { components } from './gen/schema';

type Incident = components['schemas']['Incident'];
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
  listSBOMsForComponent(componentRef: string): Promise<SBOM[]>;
  subscribeToStream(handler: (event: StreamEvent) => void): () => void;
};

export const rampartApiRef = createApiRef<RampartApi>({
  id: 'plugin.rampart.api',
});
