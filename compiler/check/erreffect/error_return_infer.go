package erreffect

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	flowpath "github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// AttachInferredErrorReturnSpec enriches function types with a canonical
// ErrorReturn effect when the function body proves the `(value, err)` pattern.
func AttachInferredErrorReturnSpec(
	fn *typ.Function,
	graph *cfg.Graph,
	solution *flow.Solution,
	synth api.Synth,
) *typ.Function {
	if fn == nil || graph == nil || synth == nil {
		return fn
	}
	base := synth.Narrow()
	if base == nil {
		base = synth
	}
	if len(fn.Returns) == 2 && !HasErrorReturnLabel(fn) &&
		HasStrictInverseReturnPattern(graph, solution, base, 0, 1) {
		fn = AttachErrorReturnSpec(fn, 0, 1)
	}
	// A multi-result function can have successful values before a trailing
	// error, including a string-valued result that makes the error position
	// ambiguous from the signature alone. Infer co-presence only when every
	// reachable return branch proves the two slots nil or present together.
	if len(fn.Returns) > 2 {
		for i := 0; i < len(fn.Returns); i++ {
			if !unwrap.IsOptionalLike(fn.Returns[i]) {
				continue
			}
			for j := i + 1; j < len(fn.Returns); j++ {
				if unwrap.IsOptionalLike(fn.Returns[j]) &&
					HasStrictSameDirectionReturnPattern(graph, solution, base, i, j) {
					fn = AttachCorrelatedReturnSpec(fn, i, j)
				}
			}
		}
	}
	return fn
}

// HasStrictSameDirectionReturnPattern proves that both result slots have the
// same nil state on every reachable explicit return, with evidence for both
// a successful and an absent pair.
func HasStrictSameDirectionReturnPattern(graph *cfg.Graph, solution *flow.Solution,
	synth api.BaseSynth, first, second int) bool {
	if graph == nil || synth == nil || first < 0 || second <= first {
		return false
	}
	var sawPresent, sawNil, incompatible bool
	graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if incompatible || info == nil || solution != nil && solution.IsPointDead(p) {
			return
		}
		if len(info.Exprs) == 0 && info.Stmt == nil {
			return
		}
		values := synth.ExpandValues(info.Exprs, second+1, p)
		if len(values) <= second {
			incompatible = true
			return
		}
		states := [2]nilState{}
		for index, slot := range []int{first, second} {
			state, ok := classifyNilState(values[slot])
			if !ok && provenPresent(graph, solution, info.Exprs, slot, p) {
				state, ok = nonNilOnly, true
			}
			if !ok && implicitReturnSlotIsNil(info.Exprs, slot) {
				state, ok = nilOnly, true
			}
			if !ok {
				incompatible = true
				return
			}
			states[index] = state
		}
		if states[0] != states[1] {
			incompatible = true
			return
		}
		sawPresent = sawPresent || states[0] == nonNilOnly
		sawNil = sawNil || states[0] == nilOnly
	})
	return !incompatible && sawPresent && sawNil
}

func AttachCorrelatedReturnSpec(fn *typ.Function, first, second int) *typ.Function {
	spec, ok := cloneContractSpec(fn)
	if !ok {
		return fn
	}
	spec.Effects = spec.Effects.With(effect.CorrelatedReturn{Indices: []int{first, second}})
	return cloneFunctionWithSpec(fn, spec)
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
	graph *cfg.Graph,
	solution *flow.Solution,
	synth api.BaseSynth,
	valueIdx int,
	errorIdx int,
) bool {
	if graph == nil || synth == nil {
		return false
	}
	var sawSuccess bool
	var sawFailure bool
	var incompatible bool
	var classified bool

	graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if incompatible || info == nil {
			return
		}
		if solution != nil && solution.IsPointDead(p) {
			return
		}
		// Skip synthetic implicit return nodes; explicit `return` without values
		// is a real nil,nil return and should block inference.
		if len(info.Exprs) == 0 && info.Stmt == nil {
			return
		}

		values := synth.ExpandValues(info.Exprs, 2, p)
		if valueIdx >= len(values) || errorIdx >= len(values) {
			incompatible = true
			return
		}

		valueState, okValue := classifyNilState(values[valueIdx])
		errorState, okError := classifyNilState(values[errorIdx])
		if !okValue && provenPresent(graph, solution, info.Exprs, valueIdx, p) {
			valueState, okValue = nonNilOnly, true
		}
		if !okError && provenPresent(graph, solution, info.Exprs, errorIdx, p) {
			errorState, okError = nonNilOnly, true
		}
		if !okValue && implicitReturnSlotIsNil(info.Exprs, valueIdx) {
			valueState, okValue = nilOnly, true
		}
		if !okError && implicitReturnSlotIsNil(info.Exprs, errorIdx) {
			errorState, okError = nilOnly, true
		}
		if !okValue || !okError {
			incompatible = true
			return
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
	})

	return classified && !incompatible && sawSuccess && sawFailure
}

// provenPresent reports whether the returned expression at idx is a path the
// flow proves present at p, for a value whose type cannot say so, such as a
// dynamic error a guard found truthy.
func provenPresent(graph *cfg.Graph, solution *flow.Solution, exprs []ast.Expr, idx int, p cfg.Point) bool {
	if solution == nil || graph == nil || idx < 0 || idx >= len(exprs) {
		return false
	}
	path := flowpath.FromExprWithBindingsAt(exprs[idx], nil, graph.Bindings(), graph, p)
	if path.IsEmpty() {
		return false
	}
	return solution.IsNonNilAt(p, path)
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
