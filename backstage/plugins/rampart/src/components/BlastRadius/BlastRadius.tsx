import { InfoCard, Link } from '@backstage/core-components';

/**
 * BlastRadius lists every component affected by an incident. Passed the
 * snapshot from IncidentDetail — the list is frozen at incident open
 * time (see engine/internal/domain/incident.go), so we do not refetch.
 */
export const BlastRadius = ({ componentRefs }: { componentRefs: string[] }) => {
  if (componentRefs.length === 0) {
    return (
      <InfoCard title="Blast radius">
        <div>No affected components.</div>
      </InfoCard>
    );
  }
  return (
    <InfoCard title={`Blast radius (${componentRefs.length})`}>
      <ul>
        {componentRefs.map(ref => (
          <li key={ref}>
            <Link to={`/catalog/${encodeURIComponent(ref)}`}>{ref}</Link>
          </li>
        ))}
      </ul>
    </InfoCard>
  );
};
