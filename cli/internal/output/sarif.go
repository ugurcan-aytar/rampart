package output

import (
	"encoding/json"
	"io"
)

// SARIF renders an SBOM as a SARIF 2.1.0 document suitable for
// github/codeql-action/upload-sarif.
//
// Scope (Phase 1): the tool block is populated (driver name / version /
// URI), but the results array is empty until IoC matching comes online.
// Package-count and ecosystem land in the run's `properties` block so a
// SARIF viewer shows something meaningful today.
type SARIF struct{}

func (SARIF) Write(w io.Writer, sbom *SBOM) error {
	doc := map[string]any{
		"version": "2.1.0",
		"$schema": "https://json.schemastore.org/sarif-2.1.0.json",
		"runs": []any{
			map[string]any{
				"tool": map[string]any{
					"driver": map[string]any{
						"name":           "rampart",
						"version":        "0.1.0",
						"informationUri": "https://github.com/ugurcan-aytar/rampart",
						"rules":          []any{},
					},
				},
				"results": []any{},
				"properties": map[string]any{
					"scanned_ecosystem":     sbom.Ecosystem,
					"scanned_source_format": sbom.SourceFormat,
					"scanned_package_count": len(sbom.Packages),
					"scanned_component_ref": sbom.ComponentRef,
				},
			},
		},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
