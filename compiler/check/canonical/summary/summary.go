// Package summary evaluates the only interprocedural semantic authority of the
// canonical type-flow engine: DAG component 10.
//
// The design target is one product equation system, not separate inner and outer
// semantic fixed points. The equation.Builder evaluates the point/demand cell
// subgraph for one function/context; the summary solve query stores the
// caller-facing Summary projection of that local solution. A recursive cluster is
// therefore a db query cycle over the same summary equations, seeded with bottom
// and accelerated by Summary widening. Diagnostic Intra reads re-run the local
// Kildall solve exactly over the converged Summary dependencies; that observer is
// not a separate memoized fixed point and cannot publish caller-visible facts.
// Termination is by domain widening plus the bottom seed, never a recursion cap
// or driver pass count.
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
//   - ReturnRefs is the finite caller-visible callable-identity tuple for returned
//     values. Each slot is keyed under the corresponding placeholder path
//     ($0, $1, ...); the caller rebases those identities onto its assignment
//     target. Function and closure identity axes stay together because call
//     outcomes and assignments consume them as one slot-relative fact.
//   - ReturnStaticMembers is the finite caller-visible static-member fact tuple
//     for returned values. Each slot contains child path facts rooted at the
//     corresponding return placeholder. The caller rebases those facts onto its
//     assignment target; product Returns still own the root value.
//   - CellEffects is the finite caller-visible capture-cell transformer. It is
//     separate from the cell store so unchanged entry cells are not mistaken for
//     writes.
//   - ReceiverEffects is the finite caller-visible runtime-argument transformer.
//     It models method/self table mutation as a summary effect applied to the
//     concrete caller argument place, instead of using prototype entry projection as
//     a post-call store update.
//   - BoundaryFacts is the finite caller-visible postcondition carrier for
//     point-local facts that are meaningful at the function boundary, such as
//     parameter/return-relative key presence, key-array provenance, and length
//     lower bounds. Product receiver effects own value shape; BoundaryFacts owns
//     path facts that must be rebased by callers.
//   - CaptureReferences is the finite lexical/reference snapshot a directly nested
//     closure may capture at entry: captured cell values plus function and closure
//     identity paths. It is store state, not a call effect; the summary solve
//     query uses it to seed child entry reference context through lexical
//     dependencies.
//   - PrototypeSelf is the finite split-pattern OOP receiver relation projected
//     from product state. It carries prototype-symbol -> runtime self value across
//     function boundaries; transfer, not summary projection, creates the relation.
//   - CallEntryPublication is caller-to-callee entry evidence projected from
//     solved call-site PointState. Values and facts are one semantic publication:
//     exact entry contexts stay exact, while aggregate caller evidence seeds
//     default contexts only through the single fixed point.
//   - Postconditions is the portable placeholder-rooted proof the function
//     publishes on every normal return. It is the only cross-boundary language
//     for callee-proven refinements of caller arguments.
//
// A Summary is the caller-facing half of the summary solve product; SummaryDomain
// gives it the lattice structure (order, join, widen) the db cycle needs to
// converge.
type Summary struct {
	// top is the SummaryDomain top sentinel. Return tuples have no finite in-band
	// top for unknown arity, so the domain needs an explicit marker just like the
	// finite map domains do. Normal projected summaries never set it.
	top                  bool
	Returns              []product.AbstractValue
	ReturnRefs           flow.ReturnRefs
	ReturnStaticMembers  []flow.StaticMemberFacts
	Params               paramevidence.Contracts
	Relations            flow.ReturnRelations
	CellEffects          flow.CaptureEffects
	ReceiverEffects      flow.ReceiverEffects
	BoundaryFacts        flow.BoundaryFacts
	CaptureReferences    flow.ReferenceContext
	PrototypeSelf        flow.PrototypeSelf
	CallEntryPublication CallEntryPublications
	Postconditions       paramevidence.ReturnPostconditions
}

type EntryValues = map[int]product.AbstractValue
type CallEntryPublication struct {
	Values EntryValues
	Facts  flow.BoundaryFacts
}
type CallEntryPublications = map[FuncRef]CallEntryPublication

// returnsDomain lifts the value-domain product over return-tuple slots: slot i
// is product.Domain, an absent (out-of-range) slot is product.Domain.Bottom(),
// and Join/Widen are slotwise over the longer arity. This is the same
// MapLattice-style pointwise lift the Env and Contracts components use, expressed
// over a positional tuple rather than a keyed map.
var returnsDomain = returnTupleLattice{}
var returnStaticMembersDomain = returnStaticMemberTupleLattice{}
var entryValuesDomain = latticeproduct.MapLattice[int](product.Domain)
var callEntryPublicationDomain = lattice.Lattice[CallEntryPublication]{
	Bottom: func() CallEntryPublication {
		return CallEntryPublication{
			Values: entryValuesDomain.Bottom(),
			Facts:  flow.BoundaryFactsDomain.Bottom(),
		}
	},
	Top: func() CallEntryPublication {
		return CallEntryPublication{
			Values: entryValuesDomain.Top(),
			Facts:  flow.BoundaryFactsDomain.Top(),
		}
	},
	Equal: func(a, b CallEntryPublication) bool {
		return entryValuesDomain.Equal(a.Values, b.Values) &&
			flow.BoundaryFactsDomain.Equal(a.Facts, b.Facts)
	},
	LessOrEq: func(a, b CallEntryPublication) bool {
		return entryValuesDomain.LessOrEq(a.Values, b.Values) &&
			flow.BoundaryFactsDomain.LessOrEq(a.Facts, b.Facts)
	},
	Join: func(a, b CallEntryPublication) CallEntryPublication {
		return CallEntryPublication{
			Values: entryValuesDomain.Join(a.Values, b.Values),
			Facts:  flow.BoundaryFactsDomain.Join(a.Facts, b.Facts),
		}
	},
	Meet: func(a, b CallEntryPublication) CallEntryPublication {
		return CallEntryPublication{
			Values: entryValuesDomain.Meet(a.Values, b.Values),
			Facts:  flow.BoundaryFactsDomain.Meet(a.Facts, b.Facts),
		}
	},
	Widen: func(prev, next CallEntryPublication) CallEntryPublication {
		return CallEntryPublication{
			Values: entryValuesDomain.Widen(prev.Values, next.Values),
			Facts:  flow.BoundaryFactsDomain.Widen(prev.Facts, next.Facts),
		}
	},
}
var callEntryPublicationsDomain = latticeproduct.MapLattice[FuncRef](callEntryPublicationDomain)
var returnPostconditionsDomain = paramevidence.ReturnPostconditionsDomain

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
			Returns:              nil,
			ReturnRefs:           flow.ReturnRefsDomain.Bottom(),
			ReturnStaticMembers:  nil,
			Params:               paramevidence.ContractDomain.Bottom(),
			Relations:            flow.ReturnRelationsDomain.Bottom(),
			CellEffects:          flow.CaptureEffectsDomain.Bottom(),
			ReceiverEffects:      flow.ReceiverEffectsDomain.Bottom(),
			BoundaryFacts:        flow.BoundaryFactsDomain.Bottom(),
			CaptureReferences:    flow.ReferenceContextDomain.Bottom(),
			PrototypeSelf:        flow.PrototypeSelfDomain.Bottom(),
			CallEntryPublication: callEntryPublicationsDomain.Bottom(),
			Postconditions:       returnPostconditionsDomain.Bottom(),
		}
	},
	Top: summaryTop,
	Equal: func(a, b Summary) bool {
		if a.top || b.top {
			return a.top && b.top
		}
		return returnsDomain.Equal(a.Returns, b.Returns) &&
			flow.ReturnRefsDomain.Equal(a.ReturnRefs, b.ReturnRefs) &&
			returnStaticMembersDomain.Equal(a.ReturnStaticMembers, b.ReturnStaticMembers) &&
			paramevidence.ContractDomain.Equal(a.Params, b.Params) &&
			flow.ReturnRelationsDomain.Equal(a.Relations, b.Relations) &&
			flow.CaptureEffectsDomain.Equal(a.CellEffects, b.CellEffects) &&
			flow.ReceiverEffectsDomain.Equal(a.ReceiverEffects, b.ReceiverEffects) &&
			flow.BoundaryFactsDomain.Equal(a.BoundaryFacts, b.BoundaryFacts) &&
			flow.ReferenceContextDomain.Equal(a.CaptureReferences, b.CaptureReferences) &&
			flow.PrototypeSelfDomain.Equal(a.PrototypeSelf, b.PrototypeSelf) &&
			callEntryPublicationsDomain.Equal(a.CallEntryPublication, b.CallEntryPublication) &&
			returnPostconditionsDomain.Equal(a.Postconditions, b.Postconditions)
	},
	LessOrEq: func(a, b Summary) bool {
		if b.top {
			return true
		}
		if a.top {
			return false
		}
		return returnsDomain.LessOrEq(a.Returns, b.Returns) &&
			flow.ReturnRefsDomain.LessOrEq(a.ReturnRefs, b.ReturnRefs) &&
			returnStaticMembersDomain.LessOrEq(a.ReturnStaticMembers, b.ReturnStaticMembers) &&
			paramevidence.ContractDomain.LessOrEq(a.Params, b.Params) &&
			flow.ReturnRelationsDomain.LessOrEq(a.Relations, b.Relations) &&
			flow.CaptureEffectsDomain.LessOrEq(a.CellEffects, b.CellEffects) &&
			flow.ReceiverEffectsDomain.LessOrEq(a.ReceiverEffects, b.ReceiverEffects) &&
			flow.BoundaryFactsDomain.LessOrEq(a.BoundaryFacts, b.BoundaryFacts) &&
			flow.ReferenceContextDomain.LessOrEq(a.CaptureReferences, b.CaptureReferences) &&
			flow.PrototypeSelfDomain.LessOrEq(a.PrototypeSelf, b.PrototypeSelf) &&
			callEntryPublicationsDomain.LessOrEq(a.CallEntryPublication, b.CallEntryPublication) &&
			returnPostconditionsDomain.LessOrEq(a.Postconditions, b.Postconditions)
	},
	Join: func(a, b Summary) Summary {
		if a.top || b.top {
			return summaryTop()
		}
		return Summary{
			Returns:              returnsDomain.Join(a.Returns, b.Returns),
			ReturnRefs:           flow.ReturnRefsDomain.Join(a.ReturnRefs, b.ReturnRefs),
			ReturnStaticMembers:  returnStaticMembersDomain.Join(a.ReturnStaticMembers, b.ReturnStaticMembers),
			Params:               paramevidence.ContractDomain.Join(a.Params, b.Params),
			Relations:            flow.ReturnRelationsDomain.Join(a.Relations, b.Relations),
			CellEffects:          flow.CaptureEffectsDomain.Join(a.CellEffects, b.CellEffects),
			ReceiverEffects:      flow.ReceiverEffectsDomain.Join(a.ReceiverEffects, b.ReceiverEffects),
			BoundaryFacts:        flow.BoundaryFactsDomain.Join(a.BoundaryFacts, b.BoundaryFacts),
			CaptureReferences:    flow.ReferenceContextDomain.Join(a.CaptureReferences, b.CaptureReferences),
			PrototypeSelf:        flow.PrototypeSelfDomain.Join(a.PrototypeSelf, b.PrototypeSelf),
			CallEntryPublication: callEntryPublicationsDomain.Join(a.CallEntryPublication, b.CallEntryPublication),
			Postconditions:       returnPostconditionsDomain.Join(a.Postconditions, b.Postconditions),
		}
	},
	Meet: nil,
	Widen: func(prev, next Summary) Summary {
		if prev.top || next.top {
			return summaryTop()
		}
		return Summary{
			Returns:              returnsDomain.Widen(prev.Returns, next.Returns),
			ReturnRefs:           flow.ReturnRefsDomain.Widen(prev.ReturnRefs, next.ReturnRefs),
			ReturnStaticMembers:  returnStaticMembersDomain.Widen(prev.ReturnStaticMembers, next.ReturnStaticMembers),
			Params:               paramevidence.ContractDomain.Widen(prev.Params, next.Params),
			Relations:            flow.ReturnRelationsDomain.Widen(prev.Relations, next.Relations),
			CellEffects:          flow.CaptureEffectsDomain.Widen(prev.CellEffects, next.CellEffects),
			ReceiverEffects:      flow.ReceiverEffectsDomain.Widen(prev.ReceiverEffects, next.ReceiverEffects),
			BoundaryFacts:        flow.BoundaryFactsDomain.Widen(prev.BoundaryFacts, next.BoundaryFacts),
			CaptureReferences:    flow.ReferenceContextDomain.Widen(prev.CaptureReferences, next.CaptureReferences),
			PrototypeSelf:        flow.PrototypeSelfDomain.Widen(prev.PrototypeSelf, next.PrototypeSelf),
			CallEntryPublication: callEntryPublicationsDomain.Widen(prev.CallEntryPublication, next.CallEntryPublication),
			Postconditions:       returnPostconditionsDomain.Widen(prev.Postconditions, next.Postconditions),
		}
	},
}

func summaryTop() Summary {
	return Summary{
		top:                  true,
		Returns:              nil,
		ReturnRefs:           flow.ReturnRefsDomain.Top(),
		ReturnStaticMembers:  nil,
		Params:               paramevidence.ContractDomain.Top(),
		Relations:            flow.ReturnRelationsDomain.Top(),
		CellEffects:          flow.CaptureEffectsDomain.Top(),
		ReceiverEffects:      flow.ReceiverEffectsDomain.Top(),
		BoundaryFacts:        flow.BoundaryFactsDomain.Top(),
		CaptureReferences:    flow.ReferenceContextDomain.Top(),
		PrototypeSelf:        flow.PrototypeSelfDomain.Top(),
		CallEntryPublication: callEntryPublicationsDomain.Top(),
		Postconditions:       returnPostconditionsDomain.Top(),
	}
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

// returnStaticMemberTupleLattice lifts StaticMemberFacts over return slots.
// Absence is Bottom so SummaryDomain.Bottom stays neutral under Join. Projection
// emits an explicit Top slot for a returned value with no child-path proof; that
// Top then drops facts not proven on every possible return path or target.
type returnStaticMemberTupleLattice struct{}

func (returnStaticMemberTupleLattice) Equal(a, b []flow.StaticMemberFacts) bool {
	n := max(len(a), len(b))
	for i := 0; i < n; i++ {
		if !flow.StaticMemberFactsDomain.Equal(staticMemberSlot(a, i), staticMemberSlot(b, i)) {
			return false
		}
	}
	return true
}

func (returnStaticMemberTupleLattice) LessOrEq(a, b []flow.StaticMemberFacts) bool {
	n := max(len(a), len(b))
	for i := 0; i < n; i++ {
		if !flow.StaticMemberFactsDomain.LessOrEq(staticMemberSlot(a, i), staticMemberSlot(b, i)) {
			return false
		}
	}
	return true
}

func (returnStaticMemberTupleLattice) Join(a, b []flow.StaticMemberFacts) []flow.StaticMemberFacts {
	return combineReturnStaticMembers(a, b, flow.StaticMemberFactsDomain.Join)
}

func (returnStaticMemberTupleLattice) Widen(prev, next []flow.StaticMemberFacts) []flow.StaticMemberFacts {
	return combineReturnStaticMembers(prev, next, flow.StaticMemberFactsDomain.Widen)
}

func combineReturnStaticMembers(
	a, b []flow.StaticMemberFacts,
	op func(flow.StaticMemberFacts, flow.StaticMemberFacts) flow.StaticMemberFacts,
) []flow.StaticMemberFacts {
	n := max(len(a), len(b))
	if n == 0 {
		return nil
	}
	out := make([]flow.StaticMemberFacts, n)
	last := -1
	for i := 0; i < n; i++ {
		out[i] = op(staticMemberSlot(a, i), staticMemberSlot(b, i))
		if !flow.StaticMemberFactsDomain.Equal(out[i], flow.StaticMemberFactsDomain.Bottom()) {
			last = i
		}
	}
	if last < 0 {
		return nil
	}
	return out[:last+1]
}

func staticMemberSlot(t []flow.StaticMemberFacts, i int) flow.StaticMemberFacts {
	if i < 0 || i >= len(t) {
		return flow.StaticMemberFactsDomain.Bottom()
	}
	return t[i]
}
