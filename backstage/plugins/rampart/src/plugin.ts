import {
  createPlugin,
  createRouteRef,
  createApiFactory,
  discoveryApiRef,
  fetchApiRef,
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
      // discoveryApi resolves `${backend.baseUrl}/api/rampart` so the
      // frontend never hits the engine directly; fetchApi threads the
      // current user's Backstage identity token through so Backstage's
      // httpRouter auth layer accepts the call.
      deps: { discoveryApi: discoveryApiRef, fetchApi: fetchApiRef },
      factory: ({ discoveryApi, fetchApi }) =>
        new RampartClient(discoveryApi, fetchApi),
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
