import { useCallback, useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Page, Header, Content, Table } from '@backstage/core-components';
import { useApi } from '@backstage/core-plugin-api';

import { rampartApiRef } from '../../api';
import { IncidentDetailDrawer } from '../IncidentDetailDrawer';
import type { components } from '../../api/gen/schema';

type Incident = components['schemas']['Incident'];

/**
 * IncidentDashboard lists every incident and updates live when the
 * engine's SSE stream fires `incident.opened`. Row click opens the
 * IncidentDetailDrawer; URL state at `?incident=<id>` makes the drawer
 * deep-linkable and survives page reloads.
 */
export const IncidentDashboard = () => {
  const api = useApi(rampartApiRef);
  const [incidents, setIncidents] = useState<Incident[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [searchParams, setSearchParams] = useSearchParams();

  const selectedIncidentId = searchParams.get('incident');

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

  useEffect(() => {
    let cancelled = false;
    api
      .listIncidents()
      .then(items => {
        if (!cancelled) setIncidents(items);
      })
      .catch((err: Error) => {
        if (!cancelled) setError(err.message);
      });

    const unsub = api.subscribeToStream(event => {
      if (event.type !== 'incident.opened') return;
      const opened = event.data as { incidentId: string; occurredAt: string; iocId: string };
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

    return () => {
      cancelled = true;
      unsub();
    };
  }, [api]);

  return (
    <Page themeId="tool">
      <Header title="Supply-chain incidents" subtitle="rampart" />
      <Content>
        {error ? <div style={{ color: 'red' }}>Error: {error}</div> : null}
        <Table
          options={{ search: true, paging: true, pageSize: 20 }}
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
