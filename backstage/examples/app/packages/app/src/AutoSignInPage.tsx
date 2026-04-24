import React from 'react';
import { SignInPage } from '@backstage/core-components';
import type { SignInPageProps } from '@backstage/core-plugin-api';

/**
 * AutoSignInPage — delegates to Backstage's canonical SignInPage with
 * `auto providers={['guest']}`, which runs the `/api/auth/guest/refresh`
 * handshake on mount. That's what produces a fully-formed IdentityApi
 * (with a real `getCredentials()` method) the way Backstage's
 * DefaultFetchApi + the rampart-backend proxy expect.
 *
 * The v0.1.x version of this file built a hand-rolled identity stub
 * (userEntityRef + profile only). That worked while RampartClient
 * spoke to the engine directly, but once the frontend started calling
 * `/api/rampart/*` through Backstage's httpRouter (Theme B1), the
 * missing `getCredentials` made every request 401.
 *
 * Production deployments that want a real auth provider swap `['guest']`
 * for their IdP (GitHub, OIDC, etc.) — Theme A3.
 */
export const AutoSignInPage = (props: SignInPageProps) => (
  <SignInPage {...props} auto providers={['guest']} />
);
