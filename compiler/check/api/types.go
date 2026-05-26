// types.go provides utility types for type synthesis operations.
package api

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// SpecTypes provides a temporary type overlay for a single synthesis operation.
// It maps symbols to types that should override the normal lookup chain without
// mutating the underlying scope or declared types.
//
// USAGE: SpecTypes is used during constraint extraction when a local narrowing
// context needs to be applied (e.g., spec-based type guards, local type assertions).
// The overlay is temporary and discarded after the operation completes.
//
// EXAMPLE: During table spec matching, SpecTypes holds the narrowed field types
// that should be visible when synthesizing the table's value expressions.
type SpecTypes = map[cfg.SymbolID]typ.Type
