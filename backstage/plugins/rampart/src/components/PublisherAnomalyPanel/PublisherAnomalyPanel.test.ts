import type { components } from '../../api/gen/schema';
import { PublisherAnomalyPanel } from './PublisherAnomalyPanel';

// Recharts' ResponsiveContainer needs ResizeObserver + a real DOM
// to mount; full visual tests live in Playwright e2e. The unit
// surface here is the prop-shape compile-time gate plus a smoke
// check that the export exists.

type IoC = components['schemas']['IoC'];

const baseIoC: IoC = {
  id: 'ioc-anomaly-1',
  kind: 'publisherAnomaly',
  severity: 'critical',
  ecosystem: 'npm',
  publishedAt: '2026-04-27T08:00:00Z',
  source: 'rampart-anomaly-orchestrator',
  anomalyBody: {
    kind: 'oidc_regression',
    confidence: 'high',
    packageRef: 'npm:axios',
    explanation: 'oidc → token regression',
    evidence: { oidc_seen_at_index: 1 },
  },
};

describe('PublisherAnomalyPanel', () => {
  it('is exported as a function component', () => {
    expect(typeof PublisherAnomalyPanel).toBe('function');
  });

  it('accepts an IoC carrying the anomalyBody variant', () => {
    const props: Parameters<typeof PublisherAnomalyPanel>[0] = { ioc: baseIoC };
    expect(props.ioc.anomalyBody?.packageRef).toBe('npm:axios');
    expect(props.ioc.anomalyBody?.kind).toBe('oidc_regression');
  });

  it('accepts the maintainer-drift kind variant', () => {
    const drift: IoC = {
      ...baseIoC,
      anomalyBody: {
        kind: 'new_maintainer_email',
        confidence: 'medium',
        packageRef: 'npm:left-pad',
        evidence: { new_emails: ['x@bad.io'] },
      },
    };
    const props: Parameters<typeof PublisherAnomalyPanel>[0] = { ioc: drift };
    expect(props.ioc.anomalyBody?.kind).toBe('new_maintainer_email');
  });

  it('accepts the version-jump kind variant', () => {
    const jump: IoC = {
      ...baseIoC,
      anomalyBody: {
        kind: 'version_jump',
        confidence: 'low',
        packageRef: 'gomod:github.com/spf13/cobra',
        evidence: { breaking_delta: 47 },
      },
    };
    const props: Parameters<typeof PublisherAnomalyPanel>[0] = { ioc: jump };
    expect(props.ioc.anomalyBody?.kind).toBe('version_jump');
  });
});
