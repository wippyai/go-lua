package body

import (
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// stateSignatureArgumentType resolves path-backed call arguments from the
// exact point state already owned by prepared body execution. Expression
// values may deliberately retain a broad literal presentation (for example,
// an empty table); signature effects need the current binding value instead.
func (s *Static) stateSignatureArgumentType(input effectlowering.SignatureOutcomeInputProgram) effectlowering.SignatureArgumentTypeProgram {
	if s == nil || s.registry == nil || !input.Valid() {
		return effectlowering.SignatureArgumentTypeProgram{}
	}
	input, err := input.WithPathValueQuery()
	if err != nil {
		// Path-value observation is optional. A restricted State lane set
		// cannot lawfully run this supplemental signature extension, so omit
		// it rather than constructing a partial query or turning a disabled
		// analysis axis into a checker panic.
		return effectlowering.SignatureArgumentTypeProgram{}
	}
	program, err := effectlowering.SealSignatureArgumentTypeProgram(input, func(ctx effectlowering.SignatureArgumentTypeContext) (typ.Type, bool) {
		source := ctx.Source
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return nil, false
		}
		p, ok := s.facts.ExpressionPath(source.ExprRef)
		if !ok || p.IsEmpty() {
			return nil, false
		}
		value, ok, queryErr := ctx.Input.PathValue(p)
		if queryErr != nil {
			panic(queryErr)
		}
		if !ok {
			return nil, false
		}
		return typevalue.TypeOf(ctx.Node.Registry, value)
	})
	if err != nil {
		panic(err)
	}
	return program
}

// SignatureArgumentTypeAtBoundary resolves a call argument through the same
// signature-argument provider used by call-outcome lowering. It is intended for
// diagnostics that need the contextual type of a function-expression argument.
func (r *Result) SignatureArgumentTypeAtBoundary(point cfg.Point, source factflow.ValueSource) (typ.Type, bool) {
	if r == nil || r.registry == nil || r.sources == nil {
		return nil, false
	}
	graph := r.Graph()
	if graph == nil {
		return nil, false
	}
	in, ok := r.solvedStateAt(point)
	if !ok {
		return nil, false
	}
	value, ok := r.sources.ValueOfSource(point, source, in, r.boundaryRead)
	if !ok {
		return nil, false
	}
	return typevalue.TypeOf(r.registry, value)
}
