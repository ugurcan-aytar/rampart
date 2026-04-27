import { useEffect, useState } from 'react';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import CircularProgress from '@mui/material/CircularProgress';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Divider from '@mui/material/Divider';
import Drawer from '@mui/material/Drawer';
import IconButton from '@mui/material/IconButton';
import Link from '@mui/material/Link';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemText from '@mui/material/ListItemText';
import Typography from '@mui/material/Typography';
import CloseIcon from '@mui/icons-material/Close';
import GraphIcon from '@mui/icons-material/AccountTree';
import { useApi } from '@backstage/core-plugin-api';

import { rampartApiRef } from '../../api';
import { BlastRadiusGraph } from '../BlastRadiusGraph';
import type { components } from '../../api/gen/schema';

type IncidentDetail = components['schemas']['IncidentDetail'];
type IoC = components['schemas']['IoC'];

/** DRAWER_WIDTH is locked at 520px — wide enough to render IoC body
 *  JSON without horizontal scroll, narrow enough to leave the
 *  IncidentDashboard table visible behind it on a 1440px laptop screen. */
const DRAWER_WIDTH = 520;

export type IncidentDetailDrawerProps = {
  /** Incident id to render. `null` keeps the drawer closed. */
  incidentId: string | null;
  /** Called when the user dismisses the drawer (X button or backdrop click). */
  onClose: () => void;
};

const DrawerHeader = ({ incidentId, onClose }: { incidentId: string | null; onClose: () => void }) => (
  <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
    <Typography variant="h6">Incident {incidentId ?? ''}</Typography>
    <IconButton onClick={onClose} aria-label="close drawer" data-testid="drawer-close">
      <CloseIcon />
    </IconButton>
  </Box>
);

/** TimelinePanel renders the incident's lifecycle in chronological order:
 *  opened → state transitions inferred from the latest snapshot. We don't
 *  yet persist a full transition log (Theme E follow-up); for now we surface
 *  Opened + LastTransitioned which captures the most recent state change. */
const TimelinePanel = ({ detail }: { detail: IncidentDetail }) => {
  const inc = detail.incident;
  const events = [
    { label: 'Opened', when: inc.openedAt, state: 'pending' },
    { label: `State → ${inc.state}`, when: inc.lastTransitionedAt, state: inc.state },
  ];
  return (
    <Box data-testid="panel-timeline">
      <Typography variant="subtitle1">Timeline</Typography>
      <List dense>
        {events.map(e => (
          <ListItem key={e.label} disableGutters>
            <ListItemText primary={e.label} secondary={e.when} />
          </ListItem>
        ))}
      </List>
    </Box>
  );
};

/** iocBodyOf returns the populated body variant — IoC is a tagged union
 *  at the wire layer, but the Go schema marshals every variant slot. */
function iocBodyOf(ioc: IoC): unknown {
  if (ioc.packageVersion) return { packageVersion: ioc.packageVersion };
  if (ioc.packageRange) return { packageRange: ioc.packageRange };
  if (ioc.publisherAnomaly) return { publisherAnomaly: ioc.publisherAnomaly };
  return {};
}

/** IoCPanel renders the matched IoC: kind, ecosystem, severity, plus a
 *  collapsible JSON dump of the body. Empty when the IoC has been deleted
 *  after the incident opened (engine returns 200 with omitted `ioc`). */
const IoCPanel = ({ detail }: { detail: IncidentDetail }) => {
  if (!detail.ioc) {
    return (
      <Box data-testid="panel-ioc">
        <Typography variant="subtitle1">Matched IoC</Typography>
        <Typography variant="body2" color="text.secondary">
          IoC no longer resolves (deleted after incident opened).
        </Typography>
      </Box>
    );
  }
  const ioc = detail.ioc;
  return (
    <Box data-testid="panel-ioc">
      <Typography variant="subtitle1">Matched IoC</Typography>
      <List dense>
        <ListItem disableGutters>
          <ListItemText primary="Id" secondary={ioc.id} />
        </ListItem>
        <ListItem disableGutters>
          <ListItemText primary="Kind" secondary={ioc.kind} />
        </ListItem>
        <ListItem disableGutters>
          <ListItemText primary="Ecosystem" secondary={ioc.ecosystem ?? '—'} />
        </ListItem>
        <ListItem disableGutters>
          <ListItemText primary="Severity" secondary={ioc.severity ?? '—'} />
        </ListItem>
        <ListItem disableGutters>
          <ListItemText primary="Source" secondary={ioc.source ?? '—'} />
        </ListItem>
      </List>
      <details>
        <summary>
          <Typography variant="caption" component="span">Body (raw JSON)</Typography>
        </summary>
        <pre style={{ fontSize: 12, overflow: 'auto', maxHeight: 200 }}>
          {JSON.stringify(iocBodyOf(ioc), null, 2)}
        </pre>
      </details>
    </Box>
  );
};

/** AffectedComponentsPanel renders the snapshot at incident-open time,
 *  hydrated by the joined endpoint. Components show ref + owner. The
 *  "Show graph" button opens BlastRadiusGraph in a full-screen dialog —
 *  drawer stays compact + mobile-friendly while the graph gets the
 *  canvas it needs. */
const AffectedComponentsPanel = ({ detail }: { detail: IncidentDetail }) => {
  const components = detail.affectedComponents ?? [];
  const [graphOpen, setGraphOpen] = useState(false);
  const [graphRoot, setGraphRoot] = useState<string | undefined>(undefined);

  return (
    <Box data-testid="panel-affected">
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <Typography variant="subtitle1">
          Affected components ({components.length})
        </Typography>
        {components.length > 0 && (
          <Button
            size="small"
            startIcon={<GraphIcon fontSize="small" />}
            onClick={() => setGraphOpen(true)}
            data-testid="open-blast-graph"
          >
            Show graph
          </Button>
        )}
      </Box>
      {components.length === 0 ? (
        <Typography variant="body2" color="text.secondary">
          No components affected (or all snapshot refs no longer resolve).
        </Typography>
      ) : (
        <List dense>
          {components.map(c => (
            <ListItem key={c.ref} disableGutters>
              <ListItemText
                primary={
                  <Link
                    href={`/catalog/${encodeURIComponent(c.ref)}`}
                    underline="hover"
                  >
                    {c.name ?? c.ref}
                  </Link>
                }
                secondary={
                  <span>
                    {c.namespace ? `${c.namespace}/` : ''}
                    owner: {c.owner ?? 'unknown'}
                  </span>
                }
              />
            </ListItem>
          ))}
        </List>
      )}
      <Dialog
        open={graphOpen}
        onClose={() => setGraphOpen(false)}
        fullWidth
        maxWidth="lg"
        data-testid="blast-graph-dialog"
      >
        <DialogTitle>
          Blast radius — Incident {detail.incident.id}
          <IconButton
            aria-label="close graph"
            onClick={() => setGraphOpen(false)}
            sx={{ position: 'absolute', right: 8, top: 8 }}
          >
            <CloseIcon />
          </IconButton>
        </DialogTitle>
        <DialogContent dividers>
          <BlastRadiusGraph
            detail={detail}
            rootComponentRef={graphRoot}
            onRootChange={setGraphRoot}
          />
        </DialogContent>
      </Dialog>
    </Box>
  );
};

/** RemediationLogPanel surfaces the append-only audit trail. Empty when
 *  no remediations have been added yet — the most common state on a
 *  freshly-opened incident. */
const RemediationLogPanel = ({ detail }: { detail: IncidentDetail }) => {
  const remediations = detail.incident.remediations ?? [];
  return (
    <Box data-testid="panel-remediations">
      <Typography variant="subtitle1">Remediations ({remediations.length})</Typography>
      {remediations.length === 0 ? (
        <Typography variant="body2" color="text.secondary">
          No remediation actions recorded yet.
        </Typography>
      ) : (
        <List dense>
          {remediations.map(r => (
            <ListItem key={r.id} disableGutters>
              <ListItemText
                primary={`${r.kind} — ${r.actorRef ?? 'unknown actor'}`}
                secondary={r.executedAt}
              />
            </ListItem>
          ))}
        </List>
      )}
    </Box>
  );
};

const DrawerBody = ({ detail }: { detail: IncidentDetail }) => (
  <>
    <TimelinePanel detail={detail} />
    <Divider sx={{ my: 2 }} />
    <IoCPanel detail={detail} />
    <Divider sx={{ my: 2 }} />
    <AffectedComponentsPanel detail={detail} />
    <Divider sx={{ my: 2 }} />
    <RemediationLogPanel detail={detail} />
  </>
);

/**
 * IncidentDetailDrawer is the right-anchored MUI Drawer the
 * IncidentDashboard opens on row click. It fetches the joined detail
 * via `getIncidentDetail` once per `incidentId` change and renders four
 * panels: Timeline, Matched IoC, Affected Components, Remediation log.
 *
 * Charts + clickable affected-component graph are deliberately out of
 * scope for this commit — see the E2 follow-up PR.
 */
export const IncidentDetailDrawer = ({ incidentId, onClose }: IncidentDetailDrawerProps) => {
  const api = useApi(rampartApiRef);
  const [detail, setDetail] = useState<IncidentDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!incidentId) {
      setDetail(null);
      setError(null);
      return undefined;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    api
      .getIncidentDetail(incidentId)
      .then(d => {
        if (!cancelled) {
          setDetail(d);
          setLoading(false);
        }
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setError(err.message);
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [api, incidentId]);

  return (
    <Drawer
      anchor="right"
      open={incidentId !== null}
      onClose={onClose}
      // MUI 9 routes paper styling through slotProps; PaperProps was the
      // pre-v6 surface and is gone from the public types.
      slotProps={{ paper: { sx: { width: DRAWER_WIDTH, padding: 2 } } }}
      data-testid="incident-detail-drawer"
    >
      <DrawerHeader incidentId={incidentId} onClose={onClose} />
      <Divider sx={{ my: 1 }} />
      {loading && <CircularProgress data-testid="drawer-loading" />}
      {error && (
        <Typography color="error" data-testid="drawer-error">
          Failed to load incident detail: {error}
        </Typography>
      )}
      {detail && !loading && !error && <DrawerBody detail={detail} />}
    </Drawer>
  );
};
