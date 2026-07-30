package sourcebridge

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
)

// ValueSourceFromASTSource lowers Lua AST provenance into the factflow source
// shape used by check-layer boundary readers.
func ValueSourceFromASTSource(source sourceprovenance.ASTSource) (factflow.ValueSource, bool) {
	if !source.Valid() {
		return factflow.ValueSource{}, false
	}
	shape, ok := factflow.NewValueSourceShape(source.Final, source.Expanded, source.Adjusted, source.OpenTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	switch source.Kind {
	case sourceprovenance.SourceCall:
		return factflow.NewCallValueSource(0, source.ExprIndex, source.TargetIndex, source.ResultIndex, source.CallPoint, shape)
	case sourceprovenance.SourceVararg:
		return factflow.NewVarargValueSource(0, source.ExprIndex, source.TargetIndex, source.ResultIndex, shape)
	case sourceprovenance.SourceNil:
		return factflow.NewNilValueSource(source.TargetIndex), true
	case sourceprovenance.SourceUnknown:
		return factflow.NewUnknownValueSource(source.TargetIndex), true
	default:
		return factflow.ValueSource{}, false
	}
}
