# @ugurcan-aytar/scaffolder-rampart-actions

Backstage scaffolder actions for [rampart](https://github.com/ugurcan-aytar/rampart)
incident remediation. Three actions wire rampart's `Remediation`
domain entity to Backstage scaffolder templates so an oncall engineer
can fix an incident from the same UI they triage it in.

> **npm publish status**: this package will appear on npm from rampart
> v0.1.1. Until then, install via the workspace path or use the
> pre-built `ghcr.io/ugurcan-aytar/rampart-backstage` container image.

## Actions

| Action | Purpose |
|---|---|
| `rampart:pin-version` | Pin a vulnerable package version in the target repository (clones, edits the lockfile, opens a PR). |
| `rampart:rotate-secret` | Trigger a secret rotation tied to a Component, via provider API, issue, or audit-log entry. |
| `rampart:open-pr` | Generic remediation PR — `.npmrc` tweak, workflow update, or any catch-all change. |

## Install

```bash
yarn add @ugurcan-aytar/scaffolder-rampart-actions
```

## Setup

In `packages/backend/src/plugins/scaffolder.ts`:

```ts
import { rampartScaffolderActions } from '@ugurcan-aytar/scaffolder-rampart-actions';

const actions = [
  ...createBuiltinActions({ /* …existing args… */ }),
  ...rampartScaffolderActions,
];
```

Or import individual actions if you prefer to register them
selectively:

```ts
import {
  pinVersionAction,
  rotateSecretAction,
  openRemediationPRAction,
} from '@ugurcan-aytar/scaffolder-rampart-actions';
```

## Example template step

```yaml
steps:
  - id: pin
    name: Pin vulnerable axios version
    action: rampart:pin-version
    input:
      componentRef: ${{ parameters.componentRef }}
      package: axios
      from: 1.11.0
      to: 1.10.5
      repoUrl: ${{ parameters.repoUrl }}
```

The action validates input against the
[`Remediation` schema](https://github.com/ugurcan-aytar/rampart/blob/main/schemas/openapi.yaml)
before executing, so an invalid template fails at scaffold-time
rather than mid-clone.

## Compatibility

| Dependency | Version |
|---|---|
| `@backstage/plugin-scaffolder-node` | `^0.4.0` |
| Node.js | `>=20` |

## License

MIT — see [LICENSE](https://github.com/ugurcan-aytar/rampart/blob/main/LICENSE).

Source and issues:
[github.com/ugurcan-aytar/rampart](https://github.com/ugurcan-aytar/rampart).
