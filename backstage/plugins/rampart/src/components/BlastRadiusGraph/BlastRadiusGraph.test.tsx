import type { components } from '../../api/gen/schema';
import { BlastRadiusGraph } from './BlastRadiusGraph';

// Reactflow renders SVG that needs jsdom + canvas + ResizeObserver
// shims to mount cleanly. Full-render tests live in Playwright e2e
// (the dev/index.tsx harness covers visual verification). Here we
// keep the unit-level guarantees minimal:
//   - the component is exported
//   - the prop type signature compiles against the generated schema
//
// If the IncidentDetail / Component schema drift, this assignment
// fails to type at backstage-cli's typecheck step.

type IncidentDetail = components['schemas']['IncidentDetail'];

const baseIncident: IncidentDetail['incident'] = {
  id: '01KQ-TEST-INCIDENT',
  iocId: 'ioc-1',
  state: 'pending',
  openedAt: '2026-04-27T08:00:00Z',
  lastTransitionedAt: '2026-04-27T08:00:00Z',
  affectedComponentsSnapshot: [],
};

describe('BlastRadiusGraph', () => {
  it('is exported as a function component', () => {
    expect(typeof BlastRadiusGraph).toBe('function');
  });

  it('accepts the documented prop shape', () => {
    const props: Parameters<typeof BlastRadiusGraph>[0] = {
      detail: { incident: baseIncident, affectedComponents: [] },
      rootComponentRef: 'kind:Component/default/web-app',
      onRootChange: () => {},
    };
    // The compile is the assertion — values just have to exist.
    expect(props.detail.incident.id).toBe('01KQ-TEST-INCIDENT');
  });
});
