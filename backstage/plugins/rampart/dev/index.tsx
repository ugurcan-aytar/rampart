import React, { useEffect } from 'react';
import { createApp } from '@backstage/app-defaults';
import { AppRouter, FlatRoutes } from '@backstage/core-app-api';
import { AlertDisplay, OAuthRequestDialog } from '@backstage/core-components';
import { Route } from 'react-router-dom';

import { rampartPlugin, IncidentDashboard } from '../src';

/**
 * Standalone dev harness. Uses createApp directly rather than
 * @backstage/dev-utils's createDevApp so we can plug an auto-completing
 * SignInPage that skips the interactive Guest-ENTER click. The click
 * adds no value for a single-plugin dev loop and prevents headless
 * browser verification — puppeteer's execution context hangs after the
 * real click fires.
 */
const AutoSignInPage = ({ onSignInSuccess }: any) => {
  useEffect(() => {
    onSignInSuccess({
      userEntityRef: 'user:default/guest',
      profile: { displayName: 'Guest', email: 'guest@example.com' },
    });
  }, [onSignInSuccess]);
  return null;
};

const app = createApp({
  apis: [],
  plugins: [rampartPlugin],
  components: {
    SignInPage: AutoSignInPage,
  },
  bindRoutes: () => {},
});

const AppRoot = app.createRoot(
  <>
    <AlertDisplay />
    <OAuthRequestDialog />
    <AppRouter>
      <FlatRoutes>
        <Route path="/rampart" element={<IncidentDashboard />} />
        <Route path="/" element={<IncidentDashboard />} />
      </FlatRoutes>
    </AppRouter>
  </>,
);

import('react-dom/client').then(ReactDOMClient => {
  const root = ReactDOMClient.createRoot(document.getElementById('root')!);
  root.render(<AppRoot />);
});
