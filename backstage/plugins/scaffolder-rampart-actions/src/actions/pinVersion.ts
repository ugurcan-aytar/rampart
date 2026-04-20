import { createTemplateAction } from '@backstage/plugin-scaffolder-node';

/**
 * rampart:pin-version
 *
 * Pins a vulnerable package to a safe version in the target repo.
 * Phase 1 skeleton: validates input + logs the intended edit. Real
 * implementation (Phase 2) clones the target repo, rewrites the
 * lockfile, and opens a PR via the GitHub API.
 */
export const pinVersionAction = createTemplateAction<{
  componentRef: string;
  packageName: string;
  fromVersion: string;
  toVersion: string;
  ecosystem?: 'npm' | 'pypi' | 'cargo' | 'go';
}>({
  id: 'rampart:pin-version',
  description: 'Pin a vulnerable package to a safe version in the target repo.',
  schema: {
    input: {
      type: 'object',
      required: ['componentRef', 'packageName', 'fromVersion', 'toVersion'],
      properties: {
        componentRef: { type: 'string', description: 'kind:Component/namespace/name' },
        packageName: { type: 'string', description: 'Package to pin (e.g. axios).' },
        fromVersion: { type: 'string', description: 'Current vulnerable version.' },
        toVersion: { type: 'string', description: 'Target safe version.' },
        ecosystem: { type: 'string', enum: ['npm', 'pypi', 'cargo', 'go'], default: 'npm' },
      },
    },
  },
  async handler(ctx) {
    const { componentRef, packageName, fromVersion, toVersion, ecosystem = 'npm' } = ctx.input;
    ctx.logger.info(
      `rampart:pin-version: would pin ${ecosystem} ${packageName}@${fromVersion} → ${toVersion} in ${componentRef}`,
    );
    // Phase 2:
    //   1. look up the Component's source repo via catalog client
    //   2. git clone, edit lockfile, commit on a branch
    //   3. open a PR via GitHub / GitLab API, tagged with the incident id
    //   4. emit a Remediation via POST /v1/incidents/{id}/remediations
  },
});
