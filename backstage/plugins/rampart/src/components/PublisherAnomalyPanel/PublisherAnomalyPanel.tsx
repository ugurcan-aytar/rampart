import { useEffect, useState } from 'react';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import CircularProgress from '@mui/material/CircularProgress';
import Divider from '@mui/material/Divider';
import Typography from '@mui/material/Typography';
import {
  Area,
  AreaChart,
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Scatter,
  ScatterChart,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { useApi } from '@backstage/core-plugin-api';

import { rampartApiRef } from '../../api';
import type { components } from '../../api/gen/schema';

type IoC = components['schemas']['IoC'];
type IoCBodyAnomaly = components['schemas']['IoCBodyAnomaly'];
type PublisherSnapshot = components['schemas']['PublisherSnapshot'];
type AnomalyKind = components['schemas']['AnomalyKind'];

export type PublisherAnomalyPanelProps = {
  ioc: IoC;
};

const KIND_LABEL: Record<AnomalyKind, string> = {
  new_maintainer_email: 'Maintainer email drift',
  oidc_regression: 'OIDC publishing regression',
  version_jump: 'Anomalous version jump',
};

const CONFIDENCE_COLOUR: Record<string, 'error' | 'warning' | 'default'> = {
  high: 'error',
  medium: 'warning',
  low: 'default',
};

// PublisherAnomalyPanel is defined at the bottom of the file so its
// child components (defined above) are in scope without forward refs.

const AnomalySummary = ({ body }: { body: IoCBodyAnomaly }) => {
  const kindLabel = KIND_LABEL[body.kind];
  const confColour = CONFIDENCE_COLOUR[body.confidence] ?? 'default';
  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
        <Typography variant="subtitle1">{kindLabel}</Typography>
        <Chip size="small" label={body.confidence} color={confColour} />
      </Box>
      <Typography variant="body2" sx={{ mb: 1 }}>
        <strong>Package:</strong> {body.packageRef}
      </Typography>
      {body.explanation && (
        <Typography variant="body2" sx={{ mb: 1 }}>
          {body.explanation}
        </Typography>
      )}
      {body.evidence && Object.keys(body.evidence).length > 0 && (
        <details>
          <summary>
            <Typography variant="caption" component="span">
              Evidence
            </Typography>
          </summary>
          <pre style={{ fontSize: 12, overflow: 'auto', maxHeight: 160 }}>
            {JSON.stringify(body.evidence, null, 2)}
          </pre>
        </details>
      )}
    </Box>
  );
};

/** MaintainerHistoryChart plots the maintainer-list size over time.
 *  Sudden drops or spikes correlate with takeover events; the count
 *  itself is a coarse signal but easy to scan. */
const MaintainerHistoryChart = ({ history }: { history: PublisherSnapshot[] }) => {
  const data = history.map(s => ({
    snapshotAt: s.snapshotAt,
    maintainers: s.maintainers?.length ?? 0,
  }));
  return (
    <Box>
      <Typography variant="caption">Maintainer count</Typography>
      <ResponsiveContainer width="100%" height={120}>
        <LineChart data={data} margin={{ top: 5, right: 10, bottom: 0, left: -20 }}>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis dataKey="snapshotAt" tickFormatter={shortTimeTick} />
          <YAxis allowDecimals={false} />
          <Tooltip />
          <Line type="stepAfter" dataKey="maintainers" stroke="#1976d2" dot={false} />
        </LineChart>
      </ResponsiveContainer>
    </Box>
  );
};

/** ReleaseCadenceChart plots version-index over time. The Y axis
 *  encodes the index of the version in the unique-versions sequence
 *  — so the slope flat-lines when the same version keeps publishing
 *  and jumps when a new release lands. */
const ReleaseCadenceChart = ({ history }: { history: PublisherSnapshot[] }) => {
  const uniqueVersions: string[] = [];
  const data = history.map(s => {
    const v = s.latestVersion ?? '';
    if (v && !uniqueVersions.includes(v)) uniqueVersions.push(v);
    return {
      snapshotAt: s.snapshotAt,
      versionIndex: v ? uniqueVersions.indexOf(v) : null,
      version: v,
    };
  });
  return (
    <Box>
      <Typography variant="caption">Release cadence</Typography>
      <ResponsiveContainer width="100%" height={120}>
        <ScatterChart margin={{ top: 5, right: 10, bottom: 0, left: -20 }}>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis dataKey="snapshotAt" tickFormatter={shortTimeTick} />
          <YAxis dataKey="versionIndex" allowDecimals={false} />
          <Tooltip />
          <Scatter data={data} fill="#ed6c02" />
        </ScatterChart>
      </ResponsiveContainer>
    </Box>
  );
};

/** OIDCStatusChart plots a binary timeline: 1 when the snapshot's
 *  publish_method is "oidc-trusted-publisher", 0 otherwise. A drop
 *  from 1→0 is the visual signature of an OIDC regression. */
const OIDCStatusChart = ({ history }: { history: PublisherSnapshot[] }) => {
  const data = history.map(s => ({
    snapshotAt: s.snapshotAt,
    oidc: s.publishMethod === 'oidc-trusted-publisher' ? 1 : 0,
  }));
  return (
    <Box>
      <Typography variant="caption">OIDC trusted publisher</Typography>
      <ResponsiveContainer width="100%" height={100}>
        <AreaChart data={data} margin={{ top: 5, right: 10, bottom: 0, left: -20 }}>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis dataKey="snapshotAt" tickFormatter={shortTimeTick} />
          <YAxis ticks={[0, 1]} domain={[0, 1]} />
          <Tooltip />
          <Area type="stepAfter" dataKey="oidc" stroke="#2e7d32" fill="#2e7d3240" />
        </AreaChart>
      </ResponsiveContainer>
    </Box>
  );
};

// shortTimeTick keeps the X axis labels readable inside the
// drawer-modal width (MM/DD form, no year, no rotation).
function shortTimeTick(value: string): string {
  if (!value) return '';
  const d = new Date(value);
  return `${String(d.getMonth() + 1).padStart(2, '0')}/${String(d.getDate()).padStart(2, '0')}`;
}

/**
 * PublisherAnomalyPanel renders an IoC carrying the IoCBodyAnomaly
 * variant (ADR-0014). The panel sits inside IncidentDetailDrawer's
 * IoC slot when `ioc.anomalyBody` is populated; the legacy
 * IoCPublisherAnomaly maintainer-keyed body falls through to the
 * standard JSON viewer.
 *
 * Three charts visualise the publisher snapshot history (fetched via
 * GET /v1/publisher/{packageRef}/history):
 *   - Maintainer count over time (LineChart)
 *   - Release cadence — version index over time (ScatterChart)
 *   - OIDC status binary timeline (AreaChart)
 *
 * History fetch is best-effort: if /v1/publisher returns empty (the
 * snapshot orchestrator is OFF — Theme F1 default), the charts
 * collapse to "No publisher history yet" placeholders. The anomaly
 * summary always renders from the IoC body alone.
 */
export const PublisherAnomalyPanel = ({ ioc }: PublisherAnomalyPanelProps) => {
  const api = useApi(rampartApiRef);
  const body = ioc.anomalyBody;
  const [history, setHistory] = useState<PublisherSnapshot[]>([]);
  const [loadingHistory, setLoadingHistory] = useState(false);
  const [historyError, setHistoryError] = useState<string | null>(null);

  useEffect(() => {
    if (!body?.packageRef) return undefined;
    let cancelled = false;
    setLoadingHistory(true);
    setHistoryError(null);
    api
      .getPublisherHistory(body.packageRef, 50)
      .then(items => {
        if (!cancelled) {
          // Snapshots arrive newest-first; charts read oldest-first.
          setHistory([...items].reverse());
          setLoadingHistory(false);
        }
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setHistoryError(err.message);
          setLoadingHistory(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [api, body?.packageRef]);

  if (!body) {
    return null;
  }

  return (
    <Box data-testid="publisher-anomaly-panel">
      <AnomalySummary body={body} />
      <Divider sx={{ my: 2 }} />
      <Typography variant="subtitle2">Publisher history</Typography>
      {loadingHistory && <CircularProgress size={18} />}
      {historyError && (
        <Typography variant="body2" color="error">
          Failed to load publisher history: {historyError}
        </Typography>
      )}
      {!loadingHistory && !historyError && history.length === 0 && (
        <Typography variant="body2" color="text.secondary">
          No publisher snapshots yet (set RAMPART_PUBLISHER_ENABLED=true to ingest).
        </Typography>
      )}
      {history.length > 0 && (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, mt: 1 }}>
          <MaintainerHistoryChart history={history} />
          <ReleaseCadenceChart history={history} />
          <OIDCStatusChart history={history} />
        </Box>
      )}
    </Box>
  );
};
