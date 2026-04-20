import React from 'react';
import { InfoCard } from '@backstage/core-components';
import { useEntity } from '@backstage/plugin-catalog-react';

const ANN_SBOM_SOURCE = 'rampart.io/sbom-source';
const ANN_LAST_SCAN = 'rampart.io/last-scan';
const ANN_CRITICALITY = 'rampart.io/criticality';

/**
 * ComponentCard goes into a Backstage Component entity page to show
 * rampart-specific metadata: SBOM source, last scan time, criticality.
 * Reads three annotations rather than hitting the engine, so the card
 * renders even if the engine is down.
 */
export const ComponentCard = () => {
  // Use the default Entity type from useEntity (real Backstage Entity
  // has required apiVersion / kind, so a bare subset type violates the
  // generic bound. Accessing annotations through entity.metadata works
  // uniformly for any Entity kind.)
  const { entity } = useEntity();
  const annotations = entity.metadata?.annotations ?? {};
  const sbomSource = annotations[ANN_SBOM_SOURCE];
  const lastScan = annotations[ANN_LAST_SCAN];
  const criticality = annotations[ANN_CRITICALITY];

  if (!sbomSource && !lastScan && !criticality) {
    return null;
  }

  return (
    <InfoCard title="rampart">
      <dl>
        {sbomSource && (
          <>
            <dt>SBOM source</dt>
            <dd>{sbomSource}</dd>
          </>
        )}
        {lastScan && (
          <>
            <dt>Last scan</dt>
            <dd>{lastScan}</dd>
          </>
        )}
        {criticality && (
          <>
            <dt>Criticality</dt>
            <dd>{criticality}</dd>
          </>
        )}
      </dl>
    </InfoCard>
  );
};
