package erreffect

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	abstractreturns "github.com/wippyai/go-lua/compiler/check/abstract/returns"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ErrorReturnConvention describes a return relation where one slot carries the
// success value and another carries the error. The convention is a pair
// relation, not a complete return-vector shape: extra return slots do not affect
// whether the value/error pair can be proven.
type ErrorReturnConvention struct {
	ValueIndex int
	ErrorIndex int
}

// CanonicalLuaValueErrorConvention returns the canonical Lua `(value, err)` layout.
func CanonicalLuaValueErrorConvention() ErrorReturnConvention {
	return ErrorReturnConvention{
		ValueIndex: 0,
		ErrorIndex: 1,
	}
}

func (c ErrorReturnConvention) valid() bool {
	return c.ValueIndex >= 0 &&
		c.ErrorIndex >= 0 &&
		c.ValueIndex != c.ErrorIndex
}

func (c ErrorReturnConvention) requiredReturnSlots() int {
	return requiredReturnSlots(c.ValueIndex, c.ErrorIndex)
}

// CanClassifyReturns reports whether returnTypes contains the slots required by
// this convention before the expensive per-return inverse-pattern proof runs.
func (c ErrorReturnConvention) CanClassifyReturns(returnTypes []typ.Type) bool {
	return c.valid() && len(returnTypes) >= c.requiredReturnSlots()
}

func (c ErrorReturnConvention) canClassifyFunction(fn *typ.Function) bool {
	return fn != nil && c.CanClassifyReturns(fn.Returns)
}

// HasStrictInversePattern proves this convention from the function body.
func (c ErrorReturnConvention) HasStrictInversePattern(
	returns []api.ReturnEvidence,
	solution *flow.Solution,
	synth abstractreturns.ExprSynth,
) bool {
	if !c.valid() {
		return false
	}
	return HasStrictInverseReturnPattern(returns, solution, synth, c.ValueIndex, c.ErrorIndex)
}

// Attach enriches fn with this convention's ErrorReturn effect.
func (c ErrorReturnConvention) Attach(fn *typ.Function) *typ.Function {
	if !c.valid() {
		return fn
	}
	return AttachErrorReturnSpec(fn, c.ValueIndex, c.ErrorIndex)
}

// AttachInferredErrorReturnSpec enriches function types with a canonical
// ErrorReturn effect when the function body proves the `(value, err)` pattern.
func AttachInferredErrorReturnSpec(
	fn *typ.Function,
	evidence api.FlowEvidence,
	solution *flow.Solution,
	synth abstractreturns.ExprSynth,
) *typ.Function {
	convention := CanonicalLuaValueErrorConvention()
	if len(evidence.Returns) == 0 || synth == nil || !convention.canClassifyFunction(fn) {
		return fn
	}
	if HasErrorReturnLabel(fn) {
		return fn
	}
	if !convention.HasStrictInversePattern(evidence.Returns, solution, synth) {
		return fn
	}

	return convention.Attach(fn)
}

func HasErrorReturnLabel(fn *typ.Function) bool {
	spec := contract.ExtractSpec(fn)
	if spec == nil {
		return false
	}
	for _, label := range spec.Effects.Labels {
		if _, ok := label.(effect.ErrorReturn); ok {
			return true
		}
	}
	return false
}

func HasStrictInverseReturnPattern(
	returns []api.ReturnEvidence,
	solution *flow.Solution,
	synth abstractreturns.ExprSynth,
	valueIdx int,
	errorIdx int,
) bool {
	if len(returns) == 0 || synth == nil {
		return false
	}
	needed := requiredReturnSlots(valueIdx, errorIdx)
	if needed == 0 {
		return false
	}
	var sawSuccess bool
	var sawFailure bool
	var incompatible bool
	var classified bool

	for _, ret := range returns {
		p := ret.Point
		info := ret.Info
		if incompatible || info == nil {
			continue
		}
		if solution != nil && solution.IsPointDead(p) {
			continue
		}
		// Skip synthetic implicit return nodes; explicit `return` without values
		// is a real nil,nil return and should block inference.
		if len(info.Exprs) == 0 && info.Stmt == nil {
			continue
		}

		if delegatesErrorReturn(info, p, synth, valueIdx, errorIdx) {
			classified = true
			sawSuccess = true
			sawFailure = true
			continue
		}

		values := abstractreturns.ExpandValues(info.Exprs, needed, p, synth)
		if valueIdx >= len(values) || errorIdx >= len(values) {
			incompatible = true
			continue
		}

		valueState, okValue := classifyNilState(values[valueIdx])
		errorState, okError := classifyNilState(values[errorIdx])
		if !okValue && implicitReturnSlotIsNil(info.Exprs, valueIdx) {
			valueState, okValue = nilOnly, true
		}
		if !okError && implicitReturnSlotIsNil(info.Exprs, errorIdx) {
			errorState, okError = nilOnly, true
		}
		if !okValue || !okError {
			incompatible = true
			continue
		}
		classified = true

		switch {
		case valueState == nilOnly && errorState == nonNilOnly:
			sawFailure = true
		case valueState == nonNilOnly && errorState == nilOnly:
			sawSuccess = true
		default:
			incompatible = true
		}
	}

	return classified && !incompatible && sawSuccess && sawFailure
}

func delegatesErrorReturn(
	info *cfg.ReturnInfo,
	p cfg.Point,
	synth abstractreturns.ExprSynth,
	valueIdx int,
	errorIdx int,
) bool {
	if info == nil || synth == nil || len(info.Exprs) != 1 {
		return false
	}
	call, ok := info.Exprs[0].(*ast.FuncCallExpr)
	if !ok || call == nil || call.Func == nil {
		return false
	}
	fn := unwrap.Function(synth.TypeOf(call.Func, p))
	if fn == nil {
		return false
	}
	spec := contract.ExtractSpec(fn)
	if spec == nil {
		return false
	}
	er := spec.Effects.GetErrorReturn(valueIdx)
	return er != nil && er.ErrorIndex == errorIdx
}

func requiredReturnSlots(valueIdx int, errorIdx int) int {
	if valueIdx < 0 || errorIdx < 0 || valueIdx == errorIdx {
		return 0
	}
	if valueIdx > errorIdx {
		return valueIdx + 1
	}
	return errorIdx + 1
}

func AttachErrorReturnSpec(fn *typ.Function, valueIndex, errorIndex int) *typ.Function {
	if fn == nil {
		return fn
	}
	if HasErrorReturnLabel(fn) {
		return fn
	}
	spec, ok := cloneContractSpec(fn)
	if !ok {
		return fn
	}
	spec.Effects = spec.Effects.With(effect.ErrorReturn{ValueIndex: valueIndex, ErrorIndex: errorIndex})
	return cloneFunctionWithSpec(fn, spec)
}

type nilState uint8

const (
	nilUnknown nilState = iota
	nilOnly
	nonNilOnly
)

func classifyNilState(t typ.Type) (nilState, bool) {
	if t == nil {
		return nilUnknown, false
	}
	u := unwrap.Alias(t)
	if u == nil {
		return nilUnknown, false
	}
	if u.Kind() == kind.Never {
		return nilUnknown, false
	}
	if u.Kind() == kind.Nil {
		return nilOnly, true
	}
	if core.ContainsNil(u) {
		return nilUnknown, false
	}
	return nonNilOnly, true
}

func cloneContractSpec(fn *typ.Function) (*contract.Spec, bool) {
	if fn == nil {
		return nil, false
	}
	if fn.Spec == nil {
		return contract.NewSpec(), true
	}
	spec := contract.ExtractSpec(fn)
	if spec == nil {
		return nil, false
	}
	clone := *spec
	return &clone, true
}

func implicitReturnSlotIsNil(exprs []ast.Expr, idx int) bool {
	if idx < 0 || idx < len(exprs) || len(exprs) == 0 {
		return false
	}
	last := exprs[len(exprs)-1]
	switch last.(type) {
	case *ast.FuncCallExpr, *ast.Comma3Expr:
		return false
	default:
		return true
	}
}

func cloneFunctionWithSpec(fn *typ.Function, spec *contract.Spec) *typ.Function {
	if fn == nil || spec == nil {
		return fn
	}
	builder := typ.Func()
	for _, tp := range fn.TypeParams {
		builder.TypeParam(tp.Name, tp.Constraint)
	}
	for _, param := range fn.Params {
		if param.Optional {
			builder.OptParam(param.Name, param.Type)
		} else {
			builder.Param(param.Name, param.Type)
		}
	}
	if fn.Variadic != nil {
		builder.Variadic(fn.Variadic)
	}
	if len(fn.Returns) > 0 {
		builder.Returns(fn.Returns...)
	}
	if fn.Effects != nil {
		builder.Effects(fn.Effects)
	}
	builder.Spec(spec)
	if fn.Refinement != nil {
		builder.WithRefinement(fn.Refinement)
	}
	return builder.Build()
}
