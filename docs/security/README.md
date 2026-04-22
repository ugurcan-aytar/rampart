# Security documentation

Reserved for the extended security narrative — threat model
(STRIDE-style analysis of how an attacker compromises rampart
itself) and the supply-chain-defence dogfooding write-up.

The directory is intentionally empty at v0.1.0. The authoritative
security reference for v0.1.0 is the root
[`SECURITY.md`](../../SECURITY.md), which covers:

- Reporting a vulnerability.
- The known operator-side gaps (in-memory storage, permissive CORS
  default, guest auth in the demo Backstage stack, no first-party
  auth on `/v1/*`).
- The cosign verification recipe for release artefacts.
- The Sigstore + GitHub OIDC trust chain that signs every release
  binary, archive, and container image.

Extended security documentation is planned for a future release.
