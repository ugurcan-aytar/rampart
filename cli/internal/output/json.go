package output

import (
	"encoding/json"
	"io"
)

// JSON renders an SBOM as indented JSON. Intended for pipelines that want
// to jq the output.
type JSON struct{}

func (JSON) Write(w io.Writer, sbom *SBOM) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(sbom)
}
