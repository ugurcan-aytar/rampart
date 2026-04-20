import { createTemplateAction } from '@backstage/plugin-scaffolder-node';

/**
 * rampart:rotate-secret
 *
 * Rotates a secret known to have been exposed by a compromised
 * maintainer account or leaked via a supply-chain incident.
 *
 * Phase 1 skeleton: validates input + logs the intended rotation.
 * Real implementation (Phase 2) integrates with the org's secret
 * manager (GitHub secrets, Vault, AWS SM) and rotates in-place.
 */
export const rotateSecretAction = createTemplateAction<{
  componentRef: string;
  secretName: string;
  rotationPolicy?: 'auto' | 'issue' | 'manual';
}>({
  id: 'rampart:rotate-secret',
  description: 'Rotate a secret tied to a Component affected by an incident.',
  schema: {
    input: {
      type: 'object',
      required: ['componentRef', 'secretName'],
      properties: {
        componentRef: { type: 'string' },
        secretName: { type: 'string', description: 'e.g. NPM_TOKEN, SLACK_WEBHOOK_URL.' },
        rotationPolicy: {
          type: 'string',
          enum: ['auto', 'issue', 'manual'],
          default: 'issue',
          description:
            'auto: rotate via provider API; issue: open a GitHub issue assigned to the owner; manual: log only.',
        },
      },
    },
  },
  async handler(ctx) {
    const { componentRef, secretName, rotationPolicy = 'issue' } = ctx.input;
    ctx.logger.info(
      `rampart:rotate-secret: would rotate ${secretName} for ${componentRef} (policy=${rotationPolicy})`,
    );
    // Phase 2: dispatch to GitHub secrets / Vault / AWS SM / issue tracker
    // and emit a Remediation on the incident.
  },
});
