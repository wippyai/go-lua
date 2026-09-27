package synth

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// FieldAccessResult contains the result of resolving a field or index access.
//
// Used by type checking to determine whether a field access is valid and
// what type it produces. The result encodes several possible outcomes:
//
//   - Found=true: Field exists, Type contains the field type
//   - Found=false, SkipCheck=true: Cannot determine validity, skip error reporting
//   - Found=false, SkipCheck=false: Field definitely missing, report error
//   - NotIndexable=true: The base type does not support indexing at all
type FieldAccessResult struct {
	Type         typ.Type
	Found        bool
	SkipCheck    bool
	NotIndexable bool
}

// Synth extends the base synthesis interface with field access resolution.
//
// This interface is implemented by Engine and provides the full synthesis
// capabilities needed by the type checker, including:
//   - All methods from api.Synth (TypeOf, MultiTypeOf, etc.)
//   - Field access resolution for attribute get expressions
//
// The ResolveFieldAccess method handles both named field access (obj.field)
// and computed index access (obj[expr]), determining validity and result type.
type Synth interface {
	api.Synth
	ResolveFieldAccess(fullExpr *ast.AttrGetExpr, objType typ.Type, fieldName string, p cfg.Point) FieldAccessResult
}
