import { createTemplateAction } from '@backstage/plugin-scaffolder-node';

/**
 * rampart:pin-version
 *
 * Pins a vulnerable package to a safe version in the target repo.
 * v0.1.0 skeleton: validates input + logs the intended edit. Real
 * implementation clones the target repo, rewrites the lockfile, and
 * opens a PR via the GitHub API.
 */
export const pinVersionAction = createTemplateAction({
  id: 'rampart:pin-version',
  description: 'Pin a vulnerable package to a safe version in the target repo.',
  schema: {
    input: {
      componentRef: z =>
        z.string().describe('kind:Component/namespace/name'),
      packageName: z =>
        z.string().describe('Package to pin (e.g. axios).'),
      fromVersion: z =>
        z.string().describe('Current vulnerable version.'),
      toVersion: z =>
        z.string().describe('Target safe version.'),
      ecosystem: z =>
        z
          .enum(['npm', 'pypi', 'cargo', 'go'])
          .default('npm')
          .describe('Package ecosystem.'),
    },
  },
  async handler(ctx) {
    const { componentRef, packageName, fromVersion, toVersion, ecosystem } =
      ctx.input;
    ctx.logger.info(
      `rampart:pin-version: would pin ${ecosystem} ${packageName}@${fromVersion} → ${toVersion} in ${componentRef}`,
    );
    // Real implementation:
    //   1. look up the Component's source repo via catalog client
    //   2. git clone, edit lockfile, commit on a branch
    //   3. open a PR via GitHub / GitLab API, tagged with the incident id
    //   4. emit a Remediation via POST /v1/incidents/{id}/remediations
  },
});
