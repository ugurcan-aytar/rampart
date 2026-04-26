package gomod

// replaceTarget is the right-hand-side of a `replace` directive in
// go.mod. NewVersion is empty for local file-system replaces (e.g.
// `replace foo => ./local`); the parser drops the source entry entirely
// for those, since there is no remote module to scan.
type replaceTarget struct {
	NewPath    string
	NewVersion string
}

// sumEntry is one physical line from go.sum after splitting and
// classifying. IsPrimary is true for the line carrying the module zip
// hash; false for the `<module> <ver>/go.mod` line that hashes go.mod
// itself.
type sumEntry struct {
	Module    string
	Version   string
	Hash      string
	IsPrimary bool
}
