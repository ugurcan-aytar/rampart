package cargo

// Lockfile mirrors the on-disk shape of Cargo.lock. Only the
// `[[package]]` array is meaningful for SBOM purposes; everything else
// (`version = N`, `[metadata]`) is tolerated and ignored.
type Lockfile struct {
	Packages []LockPackage `toml:"package"`
}

// LockPackage is a single `[[package]]` entry. A nil/missing `Source`
// indicates a workspace-local member — the project itself, not a pulled
// dep — and is filtered out.
type LockPackage struct {
	Name     string `toml:"name"`
	Version  string `toml:"version"`
	Source   string `toml:"source,omitempty"`
	Checksum string `toml:"checksum,omitempty"`
}
