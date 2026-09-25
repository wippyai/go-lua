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
	"github.com/wippyai/go-lua/types/narrow"
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
	if len(fn.Returns) == 2 &&
		(unwrap.IsOptionalLike(fn.Returns[0]) || unwrap.IsNilType(fn.Returns[0])) &&
		!HasErrorReturnLabel(fn) &&
		HasStrictInverseReturnPattern(graph, solution, base, 0, 1) {
		fn = AttachErrorReturnSpec(fn, 0, 1)
	}
	if len(fn.Returns) == 2 && solution != nil &&
		(unwrap.IsOptionalLike(fn.Returns[0]) || unwrap.IsNilType(fn.Returns[0])) &&
		HasStrictInverseReturnPattern(graph, solution, base, 0, 1) &&
		HasStrictTruthySuccessReturnPattern(graph, solution, base, 0, 1) {
		fn = attachErrorReturnSpec(fn, 0, 1, true)
	}
	if solution != nil && len(fn.Returns) > 2 {
		for first := 0; first < len(fn.Returns); first++ {
			for second := first + 1; second < len(fn.Returns); second++ {
				if (unwrap.IsOptionalLike(fn.Returns[first]) || unwrap.IsNilType(fn.Returns[first])) &&
					HasStrictInverseReturnPattern(graph, solution, base, first, second) {
					fn = AttachErrorReturnSpec(fn, first, second)
				}
			}
		}
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
	if solution != nil && len(fn.Returns) >= 2 {
		if targetType, ok := StrictTruthyReturnTargetType(graph, solution, base, 0, 1); ok {
			fn = AttachGuardedReturnTypeSpec(fn, 0, 1, targetType)
		}
	}
	return fn
}

// StrictTruthyReturnTargetType proves that every truthy guard return has the
// same concrete target type. A broad or unknown guard branch blocks the proof.
func StrictTruthyReturnTargetType(graph *cfg.Graph, solution *flow.Solution,
	synth api.BaseSynth, guardIdx, targetIdx int) (typ.Type, bool) {
	if graph == nil || solution == nil || synth == nil {
		return nil, false
	}
	var target typ.Type
	valid, sawTruthy := true, false
	graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if !valid || info == nil || solution.IsPointDead(p) {
			return
		}
		if len(info.Exprs) == 0 {
			return
		}
		values := synth.ExpandValues(info.Exprs, targetIdx+1, p)
		if len(values) <= guardIdx || len(values) <= targetIdx {
			valid = false
			return
		}
		guard := values[guardIdx]
		if guard == nil || typ.IsAny(guard) || typ.IsUnknown(guard) {
			valid = false
			return
		}
		if typ.IsNever(narrow.ToTruthy(guard)) {
			return
		}
		if !typ.IsNever(narrow.ToFalsy(guard)) {
			valid = false
			return
		}
		candidate := values[targetIdx]
		if candidate == nil || typ.IsAny(candidate) || typ.IsUnknown(candidate) || unwrap.IsOptionalLike(candidate) {
			valid = false
			return
		}
		if target != nil && !typ.TypeEquals(target, candidate) {
			valid = false
			return
		}
		target = candidate
		sawTruthy = true
	})
	if !valid || !sawTruthy || target == nil {
		return nil, false
	}
	return target, true
}

func AttachGuardedReturnTypeSpec(fn *typ.Function, guardIdx, targetIdx int, targetType typ.Type) *typ.Function {
	if fn == nil || targetType == nil {
		return fn
	}
	label := effect.GuardedReturnType{GuardIndex: guardIdx, TargetIndex: targetIdx, TargetHash: targetType.Hash(), TargetType: targetType}
	if spec := contract.ExtractSpec(fn); spec != nil {
		for _, existing := range spec.Effects.Labels {
			if existing.Equals(label) {
				return fn
			}
		}
	}
	spec, ok := cloneContractSpec(fn)
	if !ok {
		return fn
	}
	spec.Effects = spec.Effects.With(label)
	return cloneFunctionWithSpec(fn, spec)
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

// HasReturnRelationLabel reports whether the function carries a proved
// relation between its return slots.
func HasReturnRelationLabel(fn *typ.Function) bool {
	spec := contract.ExtractSpec(fn)
	if spec == nil {
		return false
	}
	for _, label := range spec.Effects.Labels {
		switch label.(type) {
		case effect.ErrorReturn, effect.CorrelatedReturn, effect.GuardedReturnType:
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
	var forwarded bool

	graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if incompatible || info == nil {
			return
		}
		if solution != nil && solution.IsPointDead(p) {
			return
		}
		// A reachable implicit return yields nil in both slots, so it blocks
		// a universal inverse relation. Pre-flow inference keeps its older
		// two-witness requirement because it has no liveness solution.
		if len(info.Exprs) == 0 && info.Stmt == nil {
			if solution != nil {
				incompatible = true
			}
			return
		}
		if forwardsErrorReturn(info.Exprs, synth, p, valueIdx, errorIdx) {
			classified = true
			forwarded = true
			return
		}

		needed := valueIdx
		if errorIdx > needed {
			needed = errorIdx
		}
		values := synth.ExpandValues(info.Exprs, needed+1, p)
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
			if !typ.IsNever(narrow.ToFalsy(values[errorIdx])) {
				incompatible = true
				return
			}
			sawFailure = true
		case valueState == nonNilOnly && errorState == nilOnly:
			sawSuccess = true
		default:
			incompatible = true
		}
	})

	return classified && !incompatible && (solution != nil || forwarded || sawSuccess && sawFailure)
}

// HasStrictTruthySuccessReturnPattern proves that every success return has a
// truthy value. It is used only together with the inverse nil-state proof.
func HasStrictTruthySuccessReturnPattern(graph *cfg.Graph, solution *flow.Solution,
	synth api.BaseSynth, valueIdx, errorIdx int) bool {
	if graph == nil || solution == nil || synth == nil {
		return false
	}
	valid, sawSuccess := true, false
	graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if !valid || info == nil || solution.IsPointDead(p) || len(info.Exprs) == 0 && info.Stmt == nil {
			return
		}
		if len(info.Exprs) == 1 {
			if call, ok := info.Exprs[0].(*ast.FuncCallExpr); ok && !call.AdjustRet {
				if allCallableAlternativesHaveTruthyErrorReturn(synth.TypeOf(call.Func, p), valueIdx, errorIdx) {
					sawSuccess = true
					return
				}
			}
		}
		values := synth.ExpandValues(info.Exprs, errorIdx+1, p)
		if len(values) <= errorIdx || len(values) <= valueIdx {
			valid = false
			return
		}
		errState, ok := classifyNilState(values[errorIdx])
		if !ok && provenPresent(graph, solution, info.Exprs, errorIdx, p) {
			errState, ok = nonNilOnly, true
		}
		if !ok && implicitReturnSlotIsNil(info.Exprs, errorIdx) {
			errState, ok = nilOnly, true
		}
		if !ok {
			valid = false
			return
		}
		if errState != nilOnly {
			return
		}
		sawSuccess = true
		if !typ.IsNever(narrow.ToFalsy(values[valueIdx])) && !provenTruthy(graph, solution, info.Exprs, valueIdx, p) {
			valid = false
		}
	})
	return valid && sawSuccess
}

func provenTruthy(graph *cfg.Graph, solution *flow.Solution, exprs []ast.Expr, idx int, p cfg.Point) bool {
	if solution == nil || graph == nil || idx < 0 || idx >= len(exprs) {
		return false
	}
	path := flowpath.FromExprWithBindingsAt(exprs[idx], nil, graph.Bindings(), graph, p)
	return !path.IsEmpty() && solution.IsTruthyAt(p, path)
}

func allCallableAlternativesHaveTruthyErrorReturn(t typ.Type, valueIdx, errorIdx int) bool {
	if t == nil {
		return false
	}
	if union, ok := typ.UnwrapAnnotated(t).(*typ.Union); ok {
		if len(union.Members) == 0 {
			return false
		}
		for _, member := range union.Members {
			if !allCallableAlternativesHaveTruthyErrorReturn(member, valueIdx, errorIdx) {
				return false
			}
		}
		return true
	}
	spec := contract.ExtractSpec(t)
	if spec == nil {
		return false
	}
	for _, label := range spec.Effects.Labels {
		if relation, ok := label.(effect.ErrorReturn); ok && relation.ValueIndex == valueIdx && relation.ErrorIndex == errorIdx && relation.ValueTruthy {
			return true
		}
	}
	return false
}

// A direct multi-result return preserves every relation guaranteed by its
// callee. It needs no independent success and failure witnesses in this body.
func forwardsErrorReturn(exprs []ast.Expr, synth api.BaseSynth, p cfg.Point, valueIdx, errorIdx int) bool {
	if len(exprs) != 1 {
		return false
	}
	call, ok := exprs[0].(*ast.FuncCallExpr)
	if !ok || call.AdjustRet || call.Func == nil {
		return false
	}
	return allCallableAlternativesHaveErrorReturn(synth.TypeOf(call.Func, p), valueIdx, errorIdx)
}

func allCallableAlternativesHaveErrorReturn(t typ.Type, valueIdx, errorIdx int) bool {
	if t == nil {
		return false
	}
	if union, ok := typ.UnwrapAnnotated(t).(*typ.Union); ok {
		if len(union.Members) == 0 {
			return false
		}
		for _, member := range union.Members {
			if !allCallableAlternativesHaveErrorReturn(member, valueIdx, errorIdx) {
				return false
			}
		}
		return true
	}
	if unwrap.Function(t) == nil {
		return false
	}
	spec := contract.ExtractSpec(t)
	if spec == nil {
		return false
	}
	for _, label := range spec.Effects.Labels {
		if relation, ok := label.(effect.ErrorReturn); ok && relation.ValueIndex == valueIdx && relation.ErrorIndex == errorIdx {
			return true
		}
	}
	return false
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
	return attachErrorReturnSpec(fn, valueIndex, errorIndex, false)
}

func attachErrorReturnSpec(fn *typ.Function, valueIndex, errorIndex int, valueTruthy bool) *typ.Function {
	if fn == nil {
		return fn
	}
	if spec := contract.ExtractSpec(fn); spec != nil {
		for _, label := range spec.Effects.Labels {
			if existing, ok := label.(effect.ErrorReturn); ok &&
				existing.ValueIndex == valueIndex && existing.ErrorIndex == errorIndex && existing.ValueTruthy == valueTruthy {
				return fn
			}
		}
	}
	spec, ok := cloneContractSpec(fn)
	if !ok {
		return fn
	}
	spec.Effects = spec.Effects.With(effect.ErrorReturn{ValueIndex: valueIndex, ErrorIndex: errorIndex, ValueTruthy: valueTruthy})
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
