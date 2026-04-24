import {
  createPlugin,
  createRouteRef,
  createApiFactory,
  discoveryApiRef,
  createRoutableExtension,
} from '@backstage/core-plugin-api';

import { rampartApiRef, RampartClient } from './api';

/** rampartRouteRef is the entry point for <Route path="/rampart"> in an app. */
export const rampartRouteRef = createRouteRef({ id: 'rampart' });

/** rampartIncidentRouteRef is the sub-route for incident detail pages. */
export const rampartIncidentRouteRef = createRouteRef({
  id: 'rampart:incident',
  params: ['id'],
});

export const rampartPlugin = createPlugin({
  id: 'rampart',
  apis: [
    createApiFactory({
      api: rampartApiRef,
      deps: { discoveryApi: discoveryApiRef },
      // RampartClient resolves `${backend.baseUrl}/api/rampart` via
      // Backstage's discoveryApi — no direct-to-engine fetches, no
      // `rampart.baseUrl` config, no browser-side CORS handshake.
      factory: ({ discoveryApi }) => new RampartClient(discoveryApi),
    }),
  ],
  routes: {
    root: rampartRouteRef,
    incident: rampartIncidentRouteRef,
  },
});

/** IncidentDashboardPage — lazy routable extension. */
export const IncidentDashboardPage = rampartPlugin.provide(
  createRoutableExtension({
    name: 'IncidentDashboardPage',
    component: () =>
      import('./components/IncidentDashboard').then(m => m.IncidentDashboard),
    mountPoint: rampartRouteRef,
  }),
);

/** IncidentDetailPage — lazy routable extension for /rampart/incidents/:id. */
export const IncidentDetailPage = rampartPlugin.provide(
  createRoutableExtension({
    name: 'IncidentDetailPage',
    component: () =>
      import('./components/IncidentDetail').then(m => m.IncidentDetail),
    mountPoint: rampartIncidentRouteRef,
  }),
);
