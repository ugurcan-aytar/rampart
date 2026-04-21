import { useEffect } from 'react';

/**
 * AutoSignInPage — Guest auto-signin with no interactive UI. Mirrors
 * the pattern in plugins/rampart/dev/index.tsx so the examples/app
 * behaves the same under headless Chrome (Playwright in e2e/).
 *
 * Production deployments that want a real auth provider replace this
 * component with Backstage's SignInPage + a configured provider (OIDC,
 * GitHub, etc.) — Phase 2.
 */
export const AutoSignInPage = ({ onSignInSuccess }: any) => {
  useEffect(() => {
    onSignInSuccess({
      userEntityRef: 'user:default/guest',
      profile: { displayName: 'Guest', email: 'guest@example.com' },
    });
  }, [onSignInSuccess]);
  return null;
};
