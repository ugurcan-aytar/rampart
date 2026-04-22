import { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import { Page, Header, Content, InfoCard, Progress } from '@backstage/core-components';
import { useApi } from '@backstage/core-plugin-api';

import { rampartApiRef } from '../../api';
import { BlastRadius } from '../BlastRadius/BlastRadius';
import type { components } from '../../api/gen/schema';

type Incident = components['schemas']['Incident'];

export const IncidentDetail = () => {
  const { id = '' } = useParams<{ id: string }>();
  const api = useApi(rampartApiRef);
  const [incident, setIncident] = useState<Incident | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    api
      .getIncident(id)
      .then(inc => {
        if (!cancelled) {
          setIncident(inc);
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
  }, [api, id]);

  if (loading) {
    return (
      <Page themeId="tool">
        <Header title="Incident" />
        <Content>
          <Progress />
        </Content>
      </Page>
    );
  }

  if (error || !incident) {
    return (
      <Page themeId="tool">
        <Header title="Incident" />
        <Content>
          <div>Could not load incident {id}: {error ?? 'not found'}</div>
        </Content>
      </Page>
    );
  }

  return (
    <Page themeId="tool">
      <Header title={`Incident ${incident.id}`} subtitle={`state: ${incident.state}`} />
      <Content>
        <InfoCard title="IoC">
          <div>{incident.iocId}</div>
        </InfoCard>
        <BlastRadius componentRefs={incident.affectedComponentsSnapshot ?? []} />
        <InfoCard title="Remediations">
          <pre>{JSON.stringify(incident.remediations ?? [], null, 2)}</pre>
        </InfoCard>
      </Content>
    </Page>
  );
};
