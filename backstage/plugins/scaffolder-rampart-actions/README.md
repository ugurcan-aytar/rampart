# @ugurcan-aytar/scaffolder-rampart-actions

Three Backstage scaffolder actions for incident remediation:

| Action                 | Purpose                                                                          |
|------------------------|----------------------------------------------------------------------------------|
| `rampart:pin-version`  | Pin a vulnerable package version in the target repo (opens a PR in Phase 2).     |
| `rampart:rotate-secret`| Rotate a secret tied to a Component — via provider API, issue, or manual log.    |
| `rampart:open-pr`      | Generic remediation PR (.npmrc tweak, workflow update, catch-all).               |

## Install

```bash
yarn add @ugurcan-aytar/scaffolder-rampart-actions
```

In `packages/backend/src/plugins/scaffolder.ts`:

```ts
import { rampartScaffolderActions } from '@ugurcan-aytar/scaffolder-rampart-actions';

const actions = [
  ...createBuiltinActions({ /* … */ }),
  ...rampartScaffolderActions,
];
```

## Status

Adım 5 iskelet. Each action validates input, logs intent, and returns.
Real side effects (clone + PR + secret rotation) land in Phase 2 when the
org has concrete VCS integrations wired.
