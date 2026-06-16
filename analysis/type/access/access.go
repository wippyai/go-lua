// Package access provides Lua table read access projections.
package access

import "github.com/wippyai/go-lua/analysis/type/typ"

// Field resolves a dot-field projection against a type.
func Field(t typ.Type, name string) (typ.Type, bool) {
	return fieldDepth(t, name, 0).materialize()
}

// MissingFieldReadsNil reports whether a missing field read on t has defined
// Lua table semantics and produces nil instead of an indexing error.
func MissingFieldReadsNil(t typ.Type) bool {
	return missingFieldReadsNilDepth(t, 0)
}
