import React from 'react';
import { Route } from 'react-router-dom';

import { createApp } from '@backstage/app-defaults';
import { AppRouter, FlatRoutes } from '@backstage/core-app-api';
import { AlertDisplay, OAuthRequestDialog } from '@backstage/core-components';

import {
  rampartPlugin,
  IncidentDashboard,
  IncidentDetail,
} from '@ugurcan-aytar/backstage-plugin-rampart';

import { AutoSignInPage } from './AutoSignInPage';

const app = createApp({
  apis: [],
  plugins: [rampartPlugin],
  components: { SignInPage: AutoSignInPage },
  bindRoutes: () => {},
});

export default app.createRoot(
  <>
    <AlertDisplay />
    <OAuthRequestDialog />
    <AppRouter>
      <FlatRoutes>
        <Route path="/rampart" element={<IncidentDashboard />} />
        <Route path="/rampart/incidents/:id" element={<IncidentDetail />} />
        <Route path="/" element={<IncidentDashboard />} />
      </FlatRoutes>
    </AppRouter>
  </>,
);
