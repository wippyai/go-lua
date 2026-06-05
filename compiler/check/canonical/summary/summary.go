// Package summary evaluates the interprocedural summary cells of the canonical
// type-flow engine: DAG component 10.
//
// The design target is one product equation system, not separate inner and outer
// semantic fixed points. The equation.Builder evaluates the point/demand cell
// subgraph for one function/context; the summary solve query stores the
// caller-facing Summary projection of that local solution. A recursive cluster is
// therefore a db query cycle over the same summary equations, seeded with bottom
// and accelerated by Summary widening. Diagnostic Intra reads re-run the local
// Kildall solve exactly over the converged Summary dependencies; that observer is
// not a separate memoized fixed point. Termination is by domain widening plus the
// bottom seed, never a recursion cap or driver pass count.
//
// Per the locked design (journal #353 / Codex C4), a summary cell must observe
// point/demand changes in the SAME db revision. The db dependency stamp is
// revision-granular: a query dependency re-fires only across a revision bump, so
// a same-revision change to a separately memoized local solve would not
// re-trigger the summary. The solve query therefore owns the local point/demand
// compute for summary projection; Intra observes the converged dependencies with
// an exact local solve so same-revision stale diagnostic states are impossible.
package summary

import (
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/lattice"
	latticeproduct "github.com/wippyai/go-lua/types/lattice/product"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// Summary is the interprocedural abstraction of one function: everything a
// caller needs and nothing about the body's internal points.
//
//   - Returns is the function's return tuple, slot i being the least upper bound
//     of the i-th returned value across every return point. An empty slice is a
//     function with no value-bearing return (or one not yet observed in the
//     fixpoint), denoting the value-domain Bottom in every slot.
//   - Params is the parameter Contracts the body imposes — the obligations a
//     caller's argument must satisfy. It is the Contracts half of the callee's
//     intraprocedural FunctionState, surfaced unchanged.
//   - Relations is the finite caller-visible return-relation component: facts
//     like Lua's `(value, err)` inverse convention that callers may consume only
//     when every return path proves them.
//   - ReturnFunctionRefs is the finite caller-visible function-identity tuple for
//     returned values. Each slot is keyed under the corresponding placeholder path
//     ($0, $1, ...); the caller rebases those identities onto its assignment target.
//   - ReturnClosureRefs is the corresponding tuple for closure values that carry
//     their lexical entry environment.
//   - CellEffects is the finite caller-visible capture-cell transformer. It is
//     separate from the cell store so unchanged entry cells are not mistaken for
//     writes.
//   - ReceiverEffects is the finite caller-visible runtime-argument transformer.
//     It models method/self table mutation as a summary effect applied to the
//     concrete caller argument place, instead of using prototype entry fallback as
//     a post-call store update.
//   - BoundaryFacts is the finite caller-visible postcondition carrier for
//     point-local facts that are meaningful at the function boundary, such as
//     parameter/return-relative key presence, key-array provenance, and length
//     lower bounds. Product receiver effects own value shape; BoundaryFacts owns
//     path facts that must be rebased by callers.
//   - CaptureExports is the finite store snapshot a directly nested closure may
//     capture at entry. It is store state, not a call effect; the summary solve
//     query uses it to seed child PointState.Cells through lexical dependencies.
//   - CaptureFunctionRefs is the finite function-identity snapshot for those same
//     captured lexical paths. The value store and identity store are separate
//     PointState axes, so Summary must export both for nested closures.
//   - CaptureClosureRefs is the finite closure-environment snapshot for captured
//     lexical paths that themselves hold closures.
//   - PrototypeSelf is the finite split-pattern OOP receiver relation projected
//     from product state. It carries prototype-symbol -> runtime self value across
//     function boundaries; transfer, not summary projection, creates the relation.
//   - CallEntryValues is caller-to-callee argument evidence projected from solved
//     call-site PointState. It is entry value state, not a public contract: the
//     summary solve query folds it into the callee's EntryValues so unannotated
//     parameters see the actual value product in the single fixed point.
//   - ParamNarrows is the finite set of parameter refinements the function proves
//     on every normal return, including wrapper effects inherited through the
//     context-free ParamNarrowQ product cell. Summary carries the cell so callers
//     observe one caller-visible function abstraction without forcing
//     bottom-context Summary dependencies.
//
// A Summary is the caller-facing half of the summary solve product; SummaryDomain
// gives it the lattice structure (order, join, widen) the db cycle needs to
// converge.
type Summary struct {
	// top is the SummaryDomain top sentinel. Return tuples have no finite in-band
	// top for unknown arity, so the domain needs an explicit marker just like the
	// finite map domains do. Normal projected summaries never set it.
	top                 bool
	Returns             []product.AbstractValue
	ReturnFunctionRefs  []flow.FunctionRefs
	ReturnClosureRefs   []flow.ClosureRefs
	Params              paramevidence.Contracts
	Relations           flow.ReturnRelations
	CellEffects         flow.CaptureEffects
	ReceiverEffects     flow.ReceiverEffects
	BoundaryFacts       flow.BoundaryFacts
	CaptureExports      flow.CaptureCells
	CaptureFunctionRefs flow.FunctionRefs
	CaptureClosureRefs  flow.ClosureRefs
	PrototypeSelf       flow.PrototypeSelf
	CallEntryValues     CallEntryValues
	ParamNarrows        []paramevidence.ParamNarrow
}

type EntryValues = map[int]product.AbstractValue
type CallEntryValues = map[FuncRef]EntryValues

// returnsDomain lifts the value-domain product over return-tuple slots: slot i
// is product.Domain, an absent (out-of-range) slot is product.Domain.Bottom(),
// and Join/Widen are slotwise over the longer arity. This is the same
// MapLattice-style pointwise lift the Env and Contracts components use, expressed
// over a positional tuple rather than a keyed map.
var returnsDomain = returnTupleLattice{}
var returnFunctionRefsDomain = returnFunctionRefsTupleLattice{}
var returnClosureRefsDomain = returnClosureRefsTupleLattice{}
var entryValuesDomain = latticeproduct.MapLattice[int](product.Domain)
var callEntryValuesDomain = latticeproduct.MapLattice[FuncRef](entryValuesDomain)
var paramNarrowsDomain = paramNarrowSetLattice{}

// SummaryDomain is the abstract domain of Summary: the componentwise reduced
// product of returnsDomain and paramevidence.ContractDomain.
//
// It is the lattice the interprocedural db cycle uses for the Summary projection.
// Equal is the convergence test for that projection; Widen is the finite-height
// accelerator that guarantees recursive summary growth terminates with a bottom
// seed. Meet is nil: both halves are forward-only (the value product and the
// contract map have no greatest lower bound), mirroring FunctionStateDomain.
var SummaryDomain = lattice.Lattice[Summary]{
	Bottom: func() Summary {
		return Summary{
			Returns:             nil,
			ReturnFunctionRefs:  nil,
			ReturnClosureRefs:   nil,
			Params:              paramevidence.ContractDomain.Bottom(),
			Relations:           flow.ReturnRelationsDomain.Bottom(),
			CellEffects:         flow.CaptureEffectsDomain.Bottom(),
			ReceiverEffects:     flow.ReceiverEffectsDomain.Bottom(),
			BoundaryFacts:       flow.BoundaryFactsDomain.Bottom(),
			CaptureExports:      flow.CaptureCellsDomain.Bottom(),
			CaptureFunctionRefs: flow.FunctionRefsDomain.Bottom(),
			CaptureClosureRefs:  flow.ClosureRefsDomain.Bottom(),
			PrototypeSelf:       flow.PrototypeSelfDomain.Bottom(),
			CallEntryValues:     callEntryValuesDomain.Bottom(),
			ParamNarrows:        paramNarrowsDomain.Bottom(),
		}
	},
	Top: summaryTop,
	Equal: func(a, b Summary) bool {
		if a.top || b.top {
			return a.top && b.top
		}
		return returnsDomain.Equal(a.Returns, b.Returns) &&
			returnFunctionRefsDomain.Equal(a.ReturnFunctionRefs, b.ReturnFunctionRefs) &&
			returnClosureRefsDomain.Equal(a.ReturnClosureRefs, b.ReturnClosureRefs) &&
			paramevidence.ContractDomain.Equal(a.Params, b.Params) &&
			flow.ReturnRelationsDomain.Equal(a.Relations, b.Relations) &&
			flow.CaptureEffectsDomain.Equal(a.CellEffects, b.CellEffects) &&
			flow.ReceiverEffectsDomain.Equal(a.ReceiverEffects, b.ReceiverEffects) &&
			flow.BoundaryFactsDomain.Equal(a.BoundaryFacts, b.BoundaryFacts) &&
			flow.CaptureCellsDomain.Equal(a.CaptureExports, b.CaptureExports) &&
			flow.FunctionRefsDomain.Equal(a.CaptureFunctionRefs, b.CaptureFunctionRefs) &&
			flow.ClosureRefsDomain.Equal(a.CaptureClosureRefs, b.CaptureClosureRefs) &&
			flow.PrototypeSelfDomain.Equal(a.PrototypeSelf, b.PrototypeSelf) &&
			callEntryValuesDomain.Equal(a.CallEntryValues, b.CallEntryValues) &&
			paramNarrowsDomain.Equal(a.ParamNarrows, b.ParamNarrows)
	},
	LessOrEq: func(a, b Summary) bool {
		if b.top {
			return true
		}
		if a.top {
			return false
		}
		return returnsDomain.LessOrEq(a.Returns, b.Returns) &&
			returnFunctionRefsDomain.LessOrEq(a.ReturnFunctionRefs, b.ReturnFunctionRefs) &&
			returnClosureRefsDomain.LessOrEq(a.ReturnClosureRefs, b.ReturnClosureRefs) &&
			paramevidence.ContractDomain.LessOrEq(a.Params, b.Params) &&
			flow.ReturnRelationsDomain.LessOrEq(a.Relations, b.Relations) &&
			flow.CaptureEffectsDomain.LessOrEq(a.CellEffects, b.CellEffects) &&
			flow.ReceiverEffectsDomain.LessOrEq(a.ReceiverEffects, b.ReceiverEffects) &&
			flow.BoundaryFactsDomain.LessOrEq(a.BoundaryFacts, b.BoundaryFacts) &&
			flow.CaptureCellsDomain.LessOrEq(a.CaptureExports, b.CaptureExports) &&
			flow.FunctionRefsDomain.LessOrEq(a.CaptureFunctionRefs, b.CaptureFunctionRefs) &&
			flow.ClosureRefsDomain.LessOrEq(a.CaptureClosureRefs, b.CaptureClosureRefs) &&
			flow.PrototypeSelfDomain.LessOrEq(a.PrototypeSelf, b.PrototypeSelf) &&
			callEntryValuesDomain.LessOrEq(a.CallEntryValues, b.CallEntryValues) &&
			paramNarrowsDomain.LessOrEq(a.ParamNarrows, b.ParamNarrows)
	},
	Join: func(a, b Summary) Summary {
		if a.top || b.top {
			return summaryTop()
		}
		return Summary{
			Returns:             returnsDomain.Join(a.Returns, b.Returns),
			ReturnFunctionRefs:  returnFunctionRefsDomain.Join(a.ReturnFunctionRefs, b.ReturnFunctionRefs),
			ReturnClosureRefs:   returnClosureRefsDomain.Join(a.ReturnClosureRefs, b.ReturnClosureRefs),
			Params:              paramevidence.ContractDomain.Join(a.Params, b.Params),
			Relations:           flow.ReturnRelationsDomain.Join(a.Relations, b.Relations),
			CellEffects:         flow.CaptureEffectsDomain.Join(a.CellEffects, b.CellEffects),
			ReceiverEffects:     flow.ReceiverEffectsDomain.Join(a.ReceiverEffects, b.ReceiverEffects),
			BoundaryFacts:       flow.BoundaryFactsDomain.Join(a.BoundaryFacts, b.BoundaryFacts),
			CaptureExports:      flow.CaptureCellsDomain.Join(a.CaptureExports, b.CaptureExports),
			CaptureFunctionRefs: flow.FunctionRefsDomain.Join(a.CaptureFunctionRefs, b.CaptureFunctionRefs),
			CaptureClosureRefs:  flow.ClosureRefsDomain.Join(a.CaptureClosureRefs, b.CaptureClosureRefs),
			PrototypeSelf:       flow.PrototypeSelfDomain.Join(a.PrototypeSelf, b.PrototypeSelf),
			CallEntryValues:     callEntryValuesDomain.Join(a.CallEntryValues, b.CallEntryValues),
			ParamNarrows:        paramNarrowsDomain.Join(a.ParamNarrows, b.ParamNarrows),
		}
	},
	Meet: nil,
	Widen: func(prev, next Summary) Summary {
		if prev.top || next.top {
			return summaryTop()
		}
		return Summary{
			Returns:             returnsDomain.Widen(prev.Returns, next.Returns),
			ReturnFunctionRefs:  returnFunctionRefsDomain.Widen(prev.ReturnFunctionRefs, next.ReturnFunctionRefs),
			ReturnClosureRefs:   returnClosureRefsDomain.Widen(prev.ReturnClosureRefs, next.ReturnClosureRefs),
			Params:              paramevidence.ContractDomain.Widen(prev.Params, next.Params),
			Relations:           flow.ReturnRelationsDomain.Widen(prev.Relations, next.Relations),
			CellEffects:         flow.CaptureEffectsDomain.Widen(prev.CellEffects, next.CellEffects),
			ReceiverEffects:     flow.ReceiverEffectsDomain.Widen(prev.ReceiverEffects, next.ReceiverEffects),
			BoundaryFacts:       flow.BoundaryFactsDomain.Widen(prev.BoundaryFacts, next.BoundaryFacts),
			CaptureExports:      flow.CaptureCellsDomain.Widen(prev.CaptureExports, next.CaptureExports),
			CaptureFunctionRefs: flow.FunctionRefsDomain.Widen(prev.CaptureFunctionRefs, next.CaptureFunctionRefs),
			CaptureClosureRefs:  flow.ClosureRefsDomain.Widen(prev.CaptureClosureRefs, next.CaptureClosureRefs),
			PrototypeSelf:       flow.PrototypeSelfDomain.Widen(prev.PrototypeSelf, next.PrototypeSelf),
			CallEntryValues:     callEntryValuesDomain.Widen(prev.CallEntryValues, next.CallEntryValues),
			ParamNarrows:        paramNarrowsDomain.Widen(prev.ParamNarrows, next.ParamNarrows),
		}
	},
}

func summaryTop() Summary {
	return Summary{
		top:                 true,
		Returns:             nil,
		ReturnFunctionRefs:  returnFunctionRefsDomain.Top(),
		ReturnClosureRefs:   returnClosureRefsDomain.Top(),
		Params:              paramevidence.ContractDomain.Top(),
		Relations:           flow.ReturnRelationsDomain.Top(),
		CellEffects:         flow.CaptureEffectsDomain.Top(),
		ReceiverEffects:     flow.ReceiverEffectsDomain.Top(),
		BoundaryFacts:       flow.BoundaryFactsDomain.Top(),
		CaptureExports:      flow.CaptureCellsDomain.Top(),
		CaptureFunctionRefs: flow.FunctionRefsDomain.Top(),
		CaptureClosureRefs:  flow.ClosureRefsDomain.Top(),
		PrototypeSelf:       flow.PrototypeSelfDomain.Top(),
		CallEntryValues:     callEntryValuesDomain.Top(),
		ParamNarrows:        paramNarrowsDomain.Top(),
	}
}

// SummaryEqual is the convergence/equality function for Summary projections.
func SummaryEqual(a, b Summary) bool { return SummaryDomain.Equal(a, b) }

// SummaryWiden is the widening the db cycle applies to a recursive Summary
// projection whose value keeps growing, so the fixpoint terminates by lattice
// height rather than a cap.
func SummaryWiden(prev, next Summary) Summary { return SummaryDomain.Widen(prev, next) }

// MergeExactOverlaySummary is the reducer for diagnostic exact-context summary
// overlays. It is deliberately not just SummaryWiden: the recursive Summary cell
// is a monotone fixed-point carrier, while the diagnostic overlay is a snapshot of
// an exact observer that may initially run before exact callees have published
// their own overlay postconditions. Value axes still use widening to terminate
// recursive exact observations. Effect and must-proof axes are latest snapshots:
// an earlier identity/no-proof observation is not a runtime path and must not
// survive after the exact callee overlay proves a write or boundary fact.
func MergeExactOverlaySummary(prev, next Summary) Summary {
	out := SummaryDomain.Widen(prev, next)
	out.Returns = mergeExactOverlayReturns(prev.Returns, next.Returns, out.Returns)
	out.CellEffects = mergeExactOverlayCellEffects(prev.CellEffects, next.CellEffects)
	out.ReceiverEffects = next.ReceiverEffects
	out.Relations = next.Relations
	out.BoundaryFacts = next.BoundaryFacts
	return out
}

func mergeExactOverlayCellEffects(prev, next flow.CaptureEffects) flow.CaptureEffects {
	if !next.IsTop() && !next.IsBottom() && len(next.Entries()) == 0 && len(prev.Entries()) > 0 {
		return prev
	}
	return next
}

func mergeExactOverlayReturns(prev, next, widened []product.AbstractValue) []product.AbstractValue {
	if len(next) == 0 || len(prev) == 0 {
		return widened
	}
	out := append([]product.AbstractValue(nil), widened...)
	for i, candidate := range next {
		if i >= len(prev) || i >= len(out) || candidate.IsZero() {
			continue
		}
		baseline := prev[i]
		if baseline.IsZero() {
			continue
		}
		if product.Domain.Equal(baseline, candidate) &&
			typ.TypeEquals(product.ProjectValueOrUnknown(baseline), product.ProjectValueOrUnknown(candidate)) {
			continue
		}
		if exactReturnValueRefines(candidate, baseline) &&
			!exactReturnValueFiniteOverWidened(baseline, candidate) &&
			((baseline.Covers(candidate) && !candidate.Covers(baseline)) ||
				exactReturnValueFiniteOverWidened(candidate, baseline)) {
			out[i] = candidate
			continue
		}
		if exactReturnValueRefines(baseline, candidate) &&
			((candidate.Covers(baseline) && !baseline.Covers(candidate)) ||
				exactReturnValueFiniteOverWidened(baseline, candidate)) {
			out[i] = baseline
		}
	}
	return out
}

func exactReturnValueFiniteOverWidened(candidate, baseline product.AbstractValue) bool {
	candidateType := candidate.ProjectValue()
	baselineType := baseline.ProjectValue()
	return candidateType != nil &&
		baselineType != nil &&
		((!typ.ContainsRecursive(candidateType) && typ.ContainsRecursive(baselineType)) ||
			(!typ.ContainsNever(candidateType) && typ.ContainsNever(baselineType)))
}

func exactReturnValueRefines(candidate, baseline product.AbstractValue) bool {
	candidateType := candidate.ProjectValue()
	baselineType := baseline.ProjectValue()
	if _, candidateLiteral := candidateType.(*typ.Literal); candidateLiteral {
		return false
	}
	if _, baselineLiteral := baselineType.(*typ.Literal); baselineLiteral {
		return false
	}
	if exactReturnValueFiniteOverWidened(candidate, baseline) {
		return true
	}
	return candidateType != nil &&
		baselineType != nil &&
		subtype.IsSubtype(candidateType, baselineType) &&
		!subtype.IsSubtype(baselineType, candidateType)
}

// JoinReturnFunctionRefs joins caller-visible returned-function identities slotwise.
// This is the public summary-owned operation for consumers that combine several
// possible callee summaries at one call site.
func JoinReturnFunctionRefs(a, b []flow.FunctionRefs) []flow.FunctionRefs {
	return returnFunctionRefsDomain.Join(a, b)
}

// JoinReturnClosureRefs joins caller-visible returned closure identities slotwise.
func JoinReturnClosureRefs(a, b []flow.ClosureRefs) []flow.ClosureRefs {
	return returnClosureRefsDomain.Join(a, b)
}

type paramNarrowSetLattice struct{}

func (paramNarrowSetLattice) Bottom() []paramevidence.ParamNarrow { return nil }

func (paramNarrowSetLattice) Top() []paramevidence.ParamNarrow { return nil }

func (paramNarrowSetLattice) Equal(a, b []paramevidence.ParamNarrow) bool {
	aa := paramevidence.SortParamNarrows(a)
	bb := paramevidence.SortParamNarrows(b)
	if len(aa) != len(bb) {
		return false
	}
	for i := range aa {
		if paramevidence.CompareParamNarrow(aa[i], bb[i]) != 0 {
			return false
		}
	}
	return true
}

func (paramNarrowSetLattice) LessOrEq(a, b []paramevidence.ParamNarrow) bool {
	aa := paramevidence.SortParamNarrows(a)
	bb := paramevidence.SortParamNarrows(b)
	j := 0
	for _, x := range aa {
		for j < len(bb) && paramevidence.CompareParamNarrow(bb[j], x) < 0 {
			j++
		}
		if j >= len(bb) || paramevidence.CompareParamNarrow(bb[j], x) != 0 {
			return false
		}
	}
	return true
}

func (paramNarrowSetLattice) Join(a, b []paramevidence.ParamNarrow) []paramevidence.ParamNarrow {
	if len(a) == 0 {
		return paramevidence.SortParamNarrows(b)
	}
	if len(b) == 0 {
		return paramevidence.SortParamNarrows(a)
	}
	out := make([]paramevidence.ParamNarrow, 0, len(a)+len(b))
	for _, e := range a {
		out = append(out, paramevidence.CloneParamNarrow(e))
	}
	for _, e := range b {
		out = append(out, paramevidence.CloneParamNarrow(e))
	}
	return paramevidence.SortParamNarrows(out)
}

func (d paramNarrowSetLattice) Widen(prev, next []paramevidence.ParamNarrow) []paramevidence.ParamNarrow {
	return d.Join(prev, next)
}

// returnTupleLattice is the value-domain product lifted positionally over a
// return tuple. It carries no state; methods operate on the slices directly.
type returnTupleLattice struct{}

func (returnTupleLattice) Equal(a, b []product.AbstractValue) bool {
	n := max(len(a), len(b))
	for i := 0; i < n; i++ {
		if !product.Domain.Equal(slot(a, i), slot(b, i)) {
			return false
		}
	}
	return true
}

func (returnTupleLattice) LessOrEq(a, b []product.AbstractValue) bool {
	n := max(len(a), len(b))
	for i := 0; i < n; i++ {
		if !product.Domain.LessOrEq(slot(a, i), slot(b, i)) {
			return false
		}
	}
	return true
}

func (returnTupleLattice) Join(a, b []product.AbstractValue) []product.AbstractValue {
	return combine(a, b, product.Domain.Join)
}

func (returnTupleLattice) Widen(prev, next []product.AbstractValue) []product.AbstractValue {
	return combine(prev, next, product.Domain.Widen)
}

// combine folds two return tuples slotwise under op over the longer arity,
// dropping a trailing run of Bottom slots so two tuples denoting the same
// function compare Equal (canonical absence == Bottom).
func combine(a, b []product.AbstractValue, op func(x, y product.AbstractValue) product.AbstractValue) []product.AbstractValue {
	n := max(len(a), len(b))
	if n == 0 {
		return nil
	}
	out := make([]product.AbstractValue, n)
	last := -1
	for i := 0; i < n; i++ {
		out[i] = op(slot(a, i), slot(b, i))
		if !product.Domain.Equal(out[i], product.Domain.Bottom()) {
			last = i
		}
	}
	if last < 0 {
		return nil
	}
	return out[:last+1]
}

// slot reads tuple slot i, returning value-domain Bottom for absence.
//
// Summary.Returns is a product-lattice carrier, not a raw storage vector. Inside
// this carrier there is one encoding for "no return fact at this slot":
// product.Domain.Bottom(). A zero AbstractValue can still leak in from older
// storage-oriented producers, but it is not a lattice element, so the tuple
// reader canonicalizes it to Bottom before Equal/Join/Widen touch the product
// domain.
func slot(t []product.AbstractValue, i int) product.AbstractValue {
	if i < 0 || i >= len(t) {
		return product.Domain.Bottom()
	}
	if t[i].IsZero() {
		return product.Domain.Bottom()
	}
	return t[i]
}

// returnFunctionRefsTupleLattice lifts FunctionRefs pointwise over return slots.
// An absent slot denotes FunctionRefs bottom, matching returnTupleLattice.
type returnFunctionRefsTupleLattice struct{}

func (returnFunctionRefsTupleLattice) Bottom() []flow.FunctionRefs { return nil }

func (returnFunctionRefsTupleLattice) Top() []flow.FunctionRefs {
	return []flow.FunctionRefs{flow.FunctionRefsDomain.Top()}
}

func (returnFunctionRefsTupleLattice) Equal(a, b []flow.FunctionRefs) bool {
	n := max(len(a), len(b))
	for i := 0; i < n; i++ {
		if !flow.FunctionRefsDomain.Equal(functionRefsSlot(a, i), functionRefsSlot(b, i)) {
			return false
		}
	}
	return true
}

func (returnFunctionRefsTupleLattice) LessOrEq(a, b []flow.FunctionRefs) bool {
	n := max(len(a), len(b))
	for i := 0; i < n; i++ {
		if !flow.FunctionRefsDomain.LessOrEq(functionRefsSlot(a, i), functionRefsSlot(b, i)) {
			return false
		}
	}
	return true
}

func (returnFunctionRefsTupleLattice) Join(a, b []flow.FunctionRefs) []flow.FunctionRefs {
	return combineFunctionRefs(a, b, flow.FunctionRefsDomain.Join)
}

func (returnFunctionRefsTupleLattice) Widen(prev, next []flow.FunctionRefs) []flow.FunctionRefs {
	return combineFunctionRefs(prev, next, flow.FunctionRefsDomain.Widen)
}

func combineFunctionRefs(a, b []flow.FunctionRefs, op func(x, y flow.FunctionRefs) flow.FunctionRefs) []flow.FunctionRefs {
	n := max(len(a), len(b))
	if n == 0 {
		return nil
	}
	out := make([]flow.FunctionRefs, n)
	last := -1
	for i := 0; i < n; i++ {
		out[i] = op(functionRefsSlot(a, i), functionRefsSlot(b, i))
		if !flow.FunctionRefsDomain.Equal(out[i], flow.FunctionRefsDomain.Bottom()) {
			last = i
		}
	}
	if last < 0 {
		return nil
	}
	return out[:last+1]
}

func functionRefsSlot(t []flow.FunctionRefs, i int) flow.FunctionRefs {
	if i < 0 || i >= len(t) {
		return flow.FunctionRefsDomain.Bottom()
	}
	return t[i]
}

// returnClosureRefsTupleLattice lifts ClosureRefs pointwise over return slots.
// An absent slot denotes ClosureRefs bottom.
type returnClosureRefsTupleLattice struct{}

func (returnClosureRefsTupleLattice) Bottom() []flow.ClosureRefs { return nil }

func (returnClosureRefsTupleLattice) Top() []flow.ClosureRefs {
	return []flow.ClosureRefs{flow.ClosureRefsDomain.Top()}
}

func (returnClosureRefsTupleLattice) Equal(a, b []flow.ClosureRefs) bool {
	n := max(len(a), len(b))
	for i := 0; i < n; i++ {
		if !flow.ClosureRefsDomain.Equal(closureRefsSlot(a, i), closureRefsSlot(b, i)) {
			return false
		}
	}
	return true
}

func (returnClosureRefsTupleLattice) LessOrEq(a, b []flow.ClosureRefs) bool {
	n := max(len(a), len(b))
	for i := 0; i < n; i++ {
		if !flow.ClosureRefsDomain.LessOrEq(closureRefsSlot(a, i), closureRefsSlot(b, i)) {
			return false
		}
	}
	return true
}

func (returnClosureRefsTupleLattice) Join(a, b []flow.ClosureRefs) []flow.ClosureRefs {
	return combineClosureRefs(a, b, flow.ClosureRefsDomain.Join)
}

func (returnClosureRefsTupleLattice) Widen(prev, next []flow.ClosureRefs) []flow.ClosureRefs {
	return combineClosureRefs(prev, next, flow.ClosureRefsDomain.Widen)
}

func combineClosureRefs(a, b []flow.ClosureRefs, op func(x, y flow.ClosureRefs) flow.ClosureRefs) []flow.ClosureRefs {
	n := max(len(a), len(b))
	if n == 0 {
		return nil
	}
	out := make([]flow.ClosureRefs, n)
	last := -1
	for i := 0; i < n; i++ {
		out[i] = op(closureRefsSlot(a, i), closureRefsSlot(b, i))
		if !flow.ClosureRefsDomain.Equal(out[i], flow.ClosureRefsDomain.Bottom()) {
			last = i
		}
	}
	if last < 0 {
		return nil
	}
	return out[:last+1]
}

func closureRefsSlot(t []flow.ClosureRefs, i int) flow.ClosureRefs {
	if i < 0 || i >= len(t) {
		return flow.ClosureRefsDomain.Bottom()
	}
	return t[i]
}
