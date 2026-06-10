package table

import "github.com/wippyai/go-lua/analysis/type/typ"

// NormalizeKey removes nil alternatives from table key domains.
func NormalizeKey(t typ.Type) typ.Type {
	if t == nil {
		return typ.Unknown
	}
	nonNil, nilable := typ.WithoutNil(t, typ.NilProjectionPreserveAliases)
	if !nilable {
		return t
	}
	if nonNil == nil {
		return typ.Never
	}
	return nonNil
}
