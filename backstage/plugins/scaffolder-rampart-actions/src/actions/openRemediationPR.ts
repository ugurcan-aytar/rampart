import { createTemplateAction } from '@backstage/plugin-scaffolder-node';

/**
 * rampart:open-pr
 *
 * Opens a generic remediation PR against a Component's source repo.
 * Templates can use this when neither pin-version nor rotate-secret fits
 * — e.g. adding a `.npmrc` entry, updating a workflow, or bumping an
 * unrelated dependency to stabilise.
 *
 * v0.1.0 skeleton: validates input + logs. Real implementation does
 * the clone + branch + commit + PR flow via the VCS integration of
 * the hosting Backstage app.
 */
export const openRemediationPRAction = createTemplateAction({
  id: 'rampart:open-pr',
  description: 'Open a remediation pull request against a Component repo.',
  schema: {
    input: {
      componentRef: z =>
        z.string().describe('kind:Component/namespace/name'),
      title: z => z.string().describe('PR title.'),
      body: z => z.string().describe('PR description (markdown).'),
      branch: z => z.string().describe('New branch name.'),
      files: z =>
        z
          .array(
            z.object({
              path: z.string(),
              content: z.string(),
            }),
          )
          .optional()
          .describe('Files to write on the new branch.'),
      labels: z =>
        z.array(z.string()).optional().describe('PR labels to apply.'),
    },
  },
  async handler(ctx) {
    const { componentRef, title, branch, files = [], labels = [] } = ctx.input;
    ctx.logger.info(
      `rampart:open-pr: would open PR "${title}" on branch ${branch} in ${componentRef} (${files.length} file(s), labels=${labels.join(',') || 'none'})`,
    );
    // Real implementation: use the scaffolder's GitRepoIntegration service
    // to clone, write files, commit, push, open PR.
  },
});
