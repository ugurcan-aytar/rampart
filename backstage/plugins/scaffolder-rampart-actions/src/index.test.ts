import {
  pinVersionAction,
  rotateSecretAction,
  openRemediationPRAction,
  rampartScaffolderActions,
} from './index';

describe('rampart scaffolder actions', () => {
  it('exposes the three actions with stable ids', () => {
    expect(pinVersionAction.id).toBe('rampart:pin-version');
    expect(rotateSecretAction.id).toBe('rampart:rotate-secret');
    expect(openRemediationPRAction.id).toBe('rampart:open-pr');
  });

  it('bundles the three actions in rampartScaffolderActions', () => {
    expect(rampartScaffolderActions).toHaveLength(3);
  });
});
