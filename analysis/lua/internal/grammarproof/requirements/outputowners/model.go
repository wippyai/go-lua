// Package outputowners generates the exact Program relation-to-owner evidence
// from the canonical Program/Target/Link relation schema.
package outputowners

import "github.com/wippyai/go-lua/analysis/schema/relations"

// Row binds one canonical Program output relation to its sole component owner.
// Output is the generated catalog name, never a source-spelled approximation.
type Row struct {
	Output string
	Owner  relations.Owner
}

// Evidence is the generated, canonical Program output-owner denominator.
type Evidence struct {
	SchemaDigest string
	Digest       string
	Rows         []Row
}

// Generated is assigned by the checked-in generated evidence source.
var Generated Evidence
