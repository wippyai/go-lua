// Package topology owns immutable canonical call-graph topology carriers.
package topology

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/types/typ"
)

// ModuleAlias records a require() alias symbol and the imported module's
// enriched export type. It is static topology, not solved point state.
type ModuleAlias struct {
	Symbol cfg.SymbolID
	Type   typ.Type
}

// FunctionBinding records that a resolved source symbol names a module-local
// function body.
type FunctionBinding struct {
	Symbol  cfg.SymbolID
	FuncRef ref.FuncRef
	Order   int
}

// FieldFunction records a statically known function stored in a table field.
// Order is deterministic discovery order and preserves first-definition-wins
// lookup for repeated static writes to the same field.
type FieldFunction struct {
	ContainerSym cfg.SymbolID
	Field        fieldkey.Key
	FuncRef      ref.FuncRef
	Order        int
}
