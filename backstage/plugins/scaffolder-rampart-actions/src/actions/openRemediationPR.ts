import { createTemplateAction } from '@backstage/plugin-scaffolder-node';

/**
 * rampart:open-pr
 *
 * Opens a generic remediation PR against a Component's source repo.
 * Templates can use this when neither pin-version nor rotate-secret fits
 * — e.g. adding a `.npmrc` entry, updating a workflow, or bumping an
 * unrelated dependency to stabilise.
 *
 * Phase 1 skeleton: validates input + logs. Phase 2 does the real
 * clone + branch + commit + PR flow via the VCS integration of the
 * hosting Backstage app.
 */
export const openRemediationPRAction = createTemplateAction<{
  componentRef: string;
  title: string;
  body: string;
  branch: string;
  files?: Array<{ path: string; content: string }>;
  labels?: string[];
}>({
  id: 'rampart:open-pr',
  description: 'Open a remediation pull request against a Component repo.',
  schema: {
    input: {
      type: 'object',
      required: ['componentRef', 'title', 'body', 'branch'],
      properties: {
        componentRef: { type: 'string' },
        title: { type: 'string' },
        body: { type: 'string', description: 'PR description (markdown).' },
        branch: { type: 'string', description: 'New branch name.' },
        files: {
          type: 'array',
          items: {
            type: 'object',
            required: ['path', 'content'],
            properties: {
              path: { type: 'string' },
              content: { type: 'string' },
            },
          },
        },
        labels: { type: 'array', items: { type: 'string' } },
      },
    },
  },
  async handler(ctx) {
    const { componentRef, title, branch, files = [], labels = [] } = ctx.input;
    ctx.logger.info(
      `rampart:open-pr: would open PR "${title}" on branch ${branch} in ${componentRef} (${files.length} file(s), labels=${labels.join(',') || 'none'})`,
    );
    // Phase 2: use the scaffolder's GitRepoIntegration service to
    // clone, write files, commit, push, open PR.
  },
});
