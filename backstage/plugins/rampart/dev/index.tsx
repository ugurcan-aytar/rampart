import React from 'react';
import { createDevApp } from '@backstage/dev-utils';

import { rampartPlugin, IncidentDashboard } from '../src';

/**
 * Standalone dev harness. Run via `yarn dev` — spins up a minimal
 * Backstage shell hosting just the rampart plugin, against whatever
 * engine URL is set in the dev config. Useful for iterating on UI
 * without firing up the full `backstage/examples/app`.
 */
createDevApp()
  .registerPlugin(rampartPlugin)
  .addPage({
    element: <IncidentDashboard />,
    title: 'Incidents',
    path: '/rampart',
  })
  .render();
