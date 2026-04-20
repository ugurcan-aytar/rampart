package npm

// Lockfile mirrors the on-disk shape of package-lock.json with
// lockfileVersion: 3. Fields outside this struct (bin, funding, …) are
// ignored — rampart only keeps what becomes an SBOM entry.
type Lockfile struct {
	Name            string                 `json:"name"`
	Version         string                 `json:"version"`
	LockfileVersion int                    `json:"lockfileVersion"`
	Requires        bool                   `json:"requires"`
	Packages        map[string]LockPackage `json:"packages"`
}

// LockPackage is a single entry in Lockfile.Packages. The key determines the
// role: "" is the project manifest; "node_modules/…" is an installed dep;
// "packages/…" is a workspace source package.
type LockPackage struct {
	Version              string            `json:"version,omitempty"`
	Resolved             string            `json:"resolved,omitempty"`
	Integrity            string            `json:"integrity,omitempty"`
	Dev                  bool              `json:"dev,omitempty"`
	Optional             bool              `json:"optional,omitempty"`
	Peer                 bool              `json:"peer,omitempty"`
	Link                 bool              `json:"link,omitempty"`
	Dependencies         map[string]string `json:"dependencies,omitempty"`
	DevDependencies      map[string]string `json:"devDependencies,omitempty"`
	PeerDependencies     map[string]string `json:"peerDependencies,omitempty"`
	OptionalDependencies map[string]string `json:"optionalDependencies,omitempty"`
	License              string            `json:"license,omitempty"`
}
