import { useCallback, useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Page, Header, Content, Table } from '@backstage/core-components';
import { useApi } from '@backstage/core-plugin-api';

import { rampartApiRef } from '../../api';
import type { IncidentListFilter } from '../../api';
import { IncidentDetailDrawer } from '../IncidentDetailDrawer';
import {
  IncidentDashboardToolbar,
  filterFromSearchParams,
  filterToSearchParams,
} from '../IncidentDashboardToolbar';
import type { components } from '../../api/gen/schema';

type Incident = components['schemas']['Incident'];

/**
 * IncidentDashboard lists every incident, exposes the IncidentDashboard
 * Toolbar for filter UI, and updates live when the engine's SSE stream
 * fires `incident.opened`. Row click opens the IncidentDetailDrawer.
 *
 * URL state holds both the active filter (state, ecosystem, search,
 * etc.) and the selected incident (`?incident=<id>`). The URL is the
 * single source of truth — page reload + back/forward navigation
 * restore filters and the open drawer.
 */
export const IncidentDashboard = () => {
  const api = useApi(rampartApiRef);
  const [incidents, setIncidents] = useState<Incident[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [searchParams, setSearchParams] = useSearchParams();

  const selectedIncidentId = searchParams.get('incident');
  const filter = useMemo<IncidentListFilter>(
    () => filterFromSearchParams(searchParams),
    [searchParams],
  );

  const setSelectedIncidentId = useCallback(
    (id: string | null) => {
      setSearchParams(prev => {
        const next = new URLSearchParams(prev);
        if (id === null) {
          next.delete('incident');
        } else {
          next.set('incident', id);
        }
        return next;
      });
    },
    [setSearchParams],
  );

  const setFilter = useCallback(
    (next: IncidentListFilter) => {
      setSearchParams(prev => filterToSearchParams(next, prev), { replace: true });
    },
    [setSearchParams],
  );

  // Refetch on filter change. The serialised JSON of the filter is
  // the dependency key — useEffect doesn't deep-compare objects, and
  // the toolbar produces a fresh object on every change.
  const filterKey = JSON.stringify(filter);
  useEffect(() => {
    let cancelled = false;
    api
      .listIncidents(filter)
      .then(items => {
        if (!cancelled) setIncidents(items);
      })
      .catch((err: Error) => {
        if (!cancelled) setError(err.message);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, filterKey]);

  // Live updates from SSE — only push to the table when the new
  // incident matches the active state filter (otherwise we'd surface
  // rows the user has already filtered out).
  useEffect(() => {
    const matchesState = (state: string) =>
      !filter.states || filter.states.length === 0 || filter.states.includes(state);
    const unsub = api.subscribeToStream(event => {
      if (event.type !== 'incident.opened') return;
      const opened = event.data as { incidentId: string; occurredAt: string; iocId: string };
      if (!matchesState('pending')) return;
      setIncidents(prev => [
        {
          id: opened.incidentId,
          state: 'pending',
          iocId: opened.iocId,
          openedAt: opened.occurredAt,
          lastTransitionedAt: opened.occurredAt,
        } as Incident,
        ...prev,
      ]);
    });
    return () => unsub();
  }, [api, filter.states]);

  return (
    <Page themeId="tool">
      <Header title="Supply-chain incidents" subtitle="rampart" />
      <Content>
        {error ? <div style={{ color: 'red' }}>Error: {error}</div> : null}
        <IncidentDashboardToolbar filter={filter} onChange={setFilter} />
        <Table
          options={{ search: false, paging: true, pageSize: 20 }}
          columns={[
            { title: 'ID', field: 'id' },
            { title: 'State', field: 'state' },
            { title: 'Opened', field: 'openedAt' },
            { title: 'IoC', field: 'iocId' },
          ]}
          data={incidents}
          title={`${incidents.length} incident(s)`}
          onRowClick={(_event, row) => {
            if (row?.id) {
              setSelectedIncidentId(row.id);
            }
          }}
        />
        <IncidentDetailDrawer
          incidentId={selectedIncidentId}
          onClose={() => setSelectedIncidentId(null)}
        />
      </Content>
    </Page>
  );
};
