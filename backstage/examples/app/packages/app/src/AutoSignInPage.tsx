import { useEffect } from 'react';

/**
 * AutoSignInPage — Guest auto-signin with no interactive UI.
 *
 * Supplies the full IdentityApi surface so Backstage's `DefaultFetchApi`
 * (and specifically `IdentityAuthInjectorFetchMiddleware`) can call
 * `getCredentials()` without throwing — the pre-B1 stub only set
 * `userEntityRef` + `profile`, which was enough while RampartClient
 * spoke to the engine directly but broke as soon as Theme B1 routed
 * traffic through Backstage's `fetchApi`.
 *
 * `getCredentials()` returns `{}` on purpose: there is no Backstage
 * user token in the guest flow (Backstage's built-in guest auth
 * backend is not wired — that's Theme A3's scope). The rampart-backend
 * plugin responds by exempting its routes via
 * `httpRouter.addAuthPolicy({ allow: 'unauthenticated' })`, so the call
 * still reaches the proxy. Engine auth (A1) is the real auth boundary
 * and is configured separately via `rampart.engine.authToken`.
 *
 * Production deployments that want a real auth provider replace this
 * component with Backstage's SignInPage + a configured provider (OIDC,
 * GitHub, etc.) — Theme A3.
 */
export const AutoSignInPage = ({ onSignInSuccess }: any) => {
  useEffect(() => {
    onSignInSuccess({
      getUserId: () => 'guest',
      getIdToken: async () => undefined,
      getProfile: () => ({
        email: 'guest@example.com',
        displayName: 'Guest',
      }),
      getProfileInfo: async () => ({
        email: 'guest@example.com',
        displayName: 'Guest',
      }),
      getBackstageIdentity: async () => ({
        type: 'user',
        userEntityRef: 'user:default/guest',
        ownershipEntityRefs: ['user:default/guest'],
      }),
      getCredentials: async () => ({}),
      signOut: async () => {},
    });
  }, [onSignInSuccess]);
  return null;
};
