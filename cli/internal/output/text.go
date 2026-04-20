package output

import (
	"fmt"
	"io"
	"strings"
)

// Text renders an SBOM as a human-readable dump. Default when --format is
// omitted. Intended for a dev scanning their own lockfile at the terminal.
type Text struct{}

func (Text) Write(w io.Writer, sbom *SBOM) error {
	fmt.Fprintf(w, "Ecosystem:      %s\n", sbom.Ecosystem)
	fmt.Fprintf(w, "Source format:  %s\n", sbom.SourceFormat)
	fmt.Fprintf(w, "Source bytes:   %d\n", sbom.SourceBytes)
	if sbom.ComponentRef != "" {
		fmt.Fprintf(w, "Component ref:  %s\n", sbom.ComponentRef)
	}
	if sbom.CommitSHA != "" {
		fmt.Fprintf(w, "Commit SHA:     %s\n", sbom.CommitSHA)
	}
	fmt.Fprintf(w, "Packages:       %d\n\n", len(sbom.Packages))
	for _, p := range sbom.Packages {
		fmt.Fprintf(w, "  %s@%s", p.Name, p.Version)
		if len(p.Scope) > 0 {
			fmt.Fprintf(w, "  [%s]", strings.Join(p.Scope, ","))
		}
		fmt.Fprintln(w)
	}
	return nil
}
