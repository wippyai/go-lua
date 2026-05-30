// Package summary computes the interprocedural fixed point of the canonical
// type-flow engine: DAG component 10.
//
// This is the OUTER of the two locked fixed points. The inner fixed point is the
// intraprocedural one — the per-function FunctionState the equation.Builder
// solves over one CFG (state.FunctionState = Points x Contracts). The outer one
// is the Sharir–Pnueli SUMMARY fixed point over the call graph: each function is
// reduced to the Summary a caller needs — its return AbstractValues and its
// parameter Contracts — and a call site inside a body resolves the callee's
// Summary rather than re-walking its body.
//
// The interprocedural recursion is NOT a third abstract domain. It is the
// least fixed point of the summary system over the call graph, computed by the
// existing db query cycle (types/db): SummaryQ recurses on itself for each callee
// a body calls; a mutually recursive cluster is detected by the db cycle
// machinery, seeded with the lattice BOTTOM summary, and iterated to a
// post-fixpoint via SummaryDomain.Widen. Termination is by the lattice
// (finite-height under widen) plus the bottom seed, never a recursion cap.
//
// Per the locked design (journal #353 / Codex C4), the interproc fixpoint must
// observe an inner intraproc change in the SAME db revision. The db dependency
// stamp is revision-granular: a query dependency re-fires only across a revision
// bump, so a same-revision change to a separately memoized inner solve would not
// re-trigger the summary. SummaryQ therefore OWNS the intraproc compute: its
// compute function drives the equation.Builder directly and reads callee
// summaries through SummaryQ itself, so the convergence of the inner solve and
// the outer summary is one db cycle with one revision-independent fixpoint.
// IntraQ memoizes that same FunctionState for callers that want the full per-
// point state without re-solving, sharing SummaryQ's cache entry shape.
package summary

import (
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
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
//
// A Summary is the value the SummaryQ cycle ranges over; SummaryDomain gives it
// the lattice structure (order, join, widen) the db cycle needs to converge.
type Summary struct {
	Returns []product.AbstractValue
	Params  paramevidence.Contracts
}

// returnsDomain lifts the value-domain product over return-tuple slots: slot i
// is product.Domain, an absent (out-of-range) slot is product.Domain.Bottom(),
// and Join/Widen are slotwise over the longer arity. This is the same
// MapLattice-style pointwise lift the Env and Contracts components use, expressed
// over a positional tuple rather than a keyed map.
var returnsDomain = returnTupleLattice{}

// SummaryDomain is the abstract domain of Summary: the componentwise reduced
// product of returnsDomain and paramevidence.ContractDomain.
//
// It is the lattice the interprocedural db cycle uses. Equal is the convergence
// test SummaryQ iterates against; Widen is the finite-height accelerator that
// guarantees the recursive summary sequence terminates with a bottom seed. Meet
// is nil: both halves are forward-only (the value product and the contract map
// have no greatest lower bound), mirroring FunctionStateDomain.
var SummaryDomain = lattice.Lattice[Summary]{
	Bottom: func() Summary {
		return Summary{
			Returns: nil,
			Params:  paramevidence.ContractDomain.Bottom(),
		}
	},
	Top: func() Summary {
		return Summary{
			Returns: nil,
			Params:  paramevidence.ContractDomain.Top(),
		}
	},
	Equal: func(a, b Summary) bool {
		return returnsDomain.Equal(a.Returns, b.Returns) &&
			paramevidence.ContractDomain.Equal(a.Params, b.Params)
	},
	LessOrEq: func(a, b Summary) bool {
		return returnsDomain.LessOrEq(a.Returns, b.Returns) &&
			paramevidence.ContractDomain.LessOrEq(a.Params, b.Params)
	},
	Join: func(a, b Summary) Summary {
		return Summary{
			Returns: returnsDomain.Join(a.Returns, b.Returns),
			Params:  paramevidence.ContractDomain.Join(a.Params, b.Params),
		}
	},
	Meet: nil,
	Widen: func(prev, next Summary) Summary {
		return Summary{
			Returns: returnsDomain.Widen(prev.Returns, next.Returns),
			Params:  paramevidence.ContractDomain.Widen(prev.Params, next.Params),
		}
	},
}

// SummaryEqual is the convergence/equality function the SummaryQ db query uses.
func SummaryEqual(a, b Summary) bool { return SummaryDomain.Equal(a, b) }

// SummaryWiden is the widening the SummaryQ db cycle applies to a recursive
// callee whose summary keeps growing, so the fixpoint terminates by lattice
// height rather than a cap.
func SummaryWiden(prev, next Summary) Summary { return SummaryDomain.Widen(prev, next) }

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

// slot reads tuple slot i, returning the value-domain Bottom for an absent slot.
func slot(t []product.AbstractValue, i int) product.AbstractValue {
	if i < 0 || i >= len(t) {
		return product.Domain.Bottom()
	}
	return t[i]
}
