import { createTemplateAction } from '@backstage/plugin-scaffolder-node';

/**
 * rampart:rotate-secret
 *
 * Rotates a secret known to have been exposed by a compromised
 * maintainer account or leaked via a supply-chain incident.
 *
 * v0.1.0 skeleton: validates input + logs the intended rotation.
 * Real implementation integrates with the org's secret manager
 * (GitHub secrets, Vault, AWS SM) and rotates in-place.
 */
export const rotateSecretAction = createTemplateAction({
  id: 'rampart:rotate-secret',
  description: 'Rotate a secret tied to a Component affected by an incident.',
  schema: {
    input: {
      componentRef: z =>
        z.string().describe('kind:Component/namespace/name'),
      secretName: z =>
        z.string().describe('e.g. NPM_TOKEN, SLACK_WEBHOOK_URL.'),
      rotationPolicy: z =>
        z
          .enum(['auto', 'issue', 'manual'])
          .default('issue')
          .describe(
            'auto: rotate via provider API; issue: open a GitHub issue assigned to the owner; manual: log only.',
          ),
    },
  },
  async handler(ctx) {
    const { componentRef, secretName, rotationPolicy } = ctx.input;
    ctx.logger.info(
      `rampart:rotate-secret: would rotate ${secretName} for ${componentRef} (policy=${rotationPolicy})`,
    );
    // Real implementation: dispatch to GitHub secrets / Vault / AWS SM /
    // issue tracker and emit a Remediation on the incident.
  },
});
