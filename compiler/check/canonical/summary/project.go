package summary

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
)

// Project reduces a solved intraprocedural FunctionState to the interprocedural
// Summary a caller consumes: the return tuple projected from the function's
// return points, paired with the parameter Contracts the body imposed.
//
// The Params half is the FunctionState.Contracts unchanged — it is already the
// caller-facing parameter obligation. The Returns half is assembled by joining,
// across every return node, the value of each returned expression read from that
// point's converged Env. A returned identifier contributes its Env value; a
// returned form whose value the intraprocedural transfer does not pin (a call
// result the transfer defers, a complex expression) contributes the value-domain
// Top in that slot, the sound over-approximation. Return arity is the widest
// return statement's expression count.
func Project(fs state.FunctionState, g *cfg.Graph) Summary {
	return Summary{
		Returns: projectReturns(fs, g),
		Params:  cloneContracts(fs.Contracts),
	}
}

// projectReturns folds the per-return-point Env into a single return tuple. For
// each return node, slot i takes the value of the i-th return expression; the
// tuple is the slotwise Join over all return points (a function with two return
// statements returns the least upper bound of both). Slots beyond a given
// statement's arity contribute Bottom for that statement.
func projectReturns(fs state.FunctionState, g *cfg.Graph) []product.AbstractValue {
	if g == nil {
		return nil
	}
	acc := returnTupleLattice{}
	var tuple []product.AbstractValue
	g.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil || len(info.Exprs) == 0 {
			return
		}
		ps, ok := fs.Points[p]
		if !ok {
			// An unreached return point (no converged state) contributes nothing;
			// its slots stay Bottom until the point becomes reachable.
			return
		}
		stmt := returnTupleAt(ps, info)
		tuple = acc.Join(tuple, stmt)
	})
	return tuple
}

// returnTupleAt reads the values of one return statement's expressions from the
// converged point state ps. A returned identifier (ReturnInfo.Symbols[i] != 0)
// projects its Env value; any other returned form whose value the transfer did
// not establish in Env projects the value-domain Top — the sound default that a
// later transfer-fidelity pass (call-return typing) refines.
func returnTupleAt(ps flow.PointState, info *cfg.ReturnInfo) []product.AbstractValue {
	out := make([]product.AbstractValue, len(info.Exprs))
	for i := range info.Exprs {
		out[i] = returnSlotValue(ps, info, i)
	}
	return out
}

func returnSlotValue(ps flow.PointState, info *cfg.ReturnInfo, i int) product.AbstractValue {
	if i < len(info.Symbols) && info.Symbols[i] != 0 {
		if av, ok := ps.Env[symKey(info.Symbols[i])]; ok && !av.IsZero() {
			return av
		}
	}
	// A non-identifier return (a literal, an arithmetic result, a call) carries its
	// value in the transfer's return-slot Env key, written by applyReturn.
	if av, ok := ps.Env[transfer.ReturnSlotKey(i)]; ok && !av.IsZero() {
		return av
	}
	return product.Domain.Top()
}

// symKey mirrors the canonical transfer's Env key convention: a parameter or
// local symbol is keyed by its CFG SymbolID. The transfer writes Env under this
// key, so the projection must read under the same key.
func symKey(sym cfg.SymbolID) string {
	return "s" + itoa(uint64(sym))
}

func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// cloneContracts copies the Contracts map so the Summary does not alias the
// FunctionState's mutable map.
func cloneContracts(c paramevidence.Contracts) paramevidence.Contracts {
	if len(c) == 0 {
		return nil
	}
	out := make(paramevidence.Contracts, len(c))
	for k, v := range c {
		out[k] = v
	}
	return out
}
