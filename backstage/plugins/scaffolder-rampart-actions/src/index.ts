import { pinVersionAction } from './actions/pinVersion';
import { rotateSecretAction } from './actions/rotateSecret';
import { openRemediationPRAction } from './actions/openRemediationPR';

export { pinVersionAction } from './actions/pinVersion';
export { rotateSecretAction } from './actions/rotateSecret';
export { openRemediationPRAction } from './actions/openRemediationPR';

/** Convenience: all rampart actions, suitable for registerScaffolderAction loops. */
export const rampartScaffolderActions = [
  pinVersionAction,
  rotateSecretAction,
  openRemediationPRAction,
];
