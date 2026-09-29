// spec_cases.go derives conditional return cases for local functions that
// dispatch on literal parameter values.
//
// After a scope group's SCCs converge, a function whose body compares a
// parameter against literal values gets one return case per distinct literal:
// rerunning body return inference with the parameter fixed to the literal
// yields the precise return type for calls passing that literal. Cases refine
// the converged default summary and never feed back into the fixpoint.
package infer

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
)

// specMemoKey identifies a cached per-literal return inference.
type specMemoKey struct {
	sym cfg.SymbolID
	idx int
	lit uint64
}

// dispatchSite is one distinct literal compared against one parameter.
type dispatchSite struct {
	paramIdx int
	sym      cfg.SymbolID
	lit      *typ.Literal
}

type dispatchReturnCase struct {
	site    dispatchSite
	returns []typ.Type
}

// buildBodyDerivedReturnCases derives literal dispatch cases for eligible local functions.
func (i *Inferencer) buildBodyDerivedReturnCases(
	run RunContext,
	sccs [][]cfg.SymbolID,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	summaries map[cfg.SymbolID][]typ.Type,
) map[cfg.SymbolID][]dispatchReturnCase {
	skip := recursiveMembers(i, sccs)
	var out map[cfg.SymbolID][]dispatchReturnCase
	for _, sym := range cfg.SortedSymbolIDs(localFuncs) {
		if skip[sym] {
			continue
		}
		info := localFuncs[sym]
		if info == nil || info.Fn == nil || info.Graph == nil {
			continue
		}
		if len(info.Fn.ReturnTypes) > 0 {
			continue
		}
		summary := summaries[sym]
		if len(summary) == 0 {
			continue
		}
		cases := i.bodyReturnCases(run, info, summaries, localFuncs)
		if len(cases) == 0 {
			continue
		}
		if out == nil {
			out = make(map[cfg.SymbolID][]dispatchReturnCase)
		}
		out[sym] = cases
	}
	return out
}

// recursiveMembers collects symbols whose SCC is recursive.
func recursiveMembers(i *Inferencer, sccs [][]cfg.SymbolID) map[cfg.SymbolID]bool {
	skip := make(map[cfg.SymbolID]bool)
	for _, scc := range sccs {
		if len(scc) > 1 {
			for _, sym := range scc {
				skip[sym] = true
			}
			continue
		}
		if len(scc) == 1 && i.recursive(scc[0]) {
			skip[scc[0]] = true
		}
	}
	return skip
}

// bodyReturnCases builds overload cases for one function from its literal
// parameter dispatches.
func (i *Inferencer) bodyReturnCases(
	run RunContext,
	info *returns.LocalFuncInfo,
	summaries map[cfg.SymbolID][]typ.Type,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
) []dispatchReturnCase {
	ctx := i.setupReturnContext(run, info, summaries, localFuncs)
	if ctx == nil {
		return nil
	}
	overlay := i.buildParameterOverlay(ctx)
	finalOverlay, untypedCapture := i.finalizeReturnOverlay(ctx, overlay)

	_, _, extractOut := i.extractForReturn(ctx, finalOverlay)
	if extractOut.Inputs == nil {
		return nil
	}
	sites := collectLiteralDispatches(info.Graph, extractOut.Inputs)
	if len(sites) == 0 {
		return nil
	}

	var cases []dispatchReturnCase
	for _, site := range sites {
		rets := i.specCaseType(ctx, untypedCapture, finalOverlay, info.Sym, site)
		if len(rets) == 0 {
			continue
		}
		cases = append(cases, dispatchReturnCase{site: site, returns: rets})
	}
	return cases
}

// specCaseType infers a return vector with one parameter fixed to a literal.
func (i *Inferencer) specCaseType(
	ctx *returnInferenceContext,
	untypedCapture bool,
	finalOverlay map[cfg.SymbolID]typ.Type,
	sym cfg.SymbolID,
	site dispatchSite,
) []typ.Type {
	key := specMemoKey{sym: sym, idx: site.paramIdx, lit: site.lit.Hash()}
	rets, ok := i.specMemo[key]
	if !ok {
		overlay := make(map[cfg.SymbolID]typ.Type, len(finalOverlay)+1)
		for k, v := range finalOverlay {
			overlay[k] = v
		}
		overlay[site.sym] = site.lit
		rets = i.inferReturnTypesFromBody(ctx, overlay)
		if untypedCapture && len(rets) > 0 {
			rets = typ.UnknownReturns(len(rets))
		}
		if i.specMemo == nil {
			i.specMemo = make(map[specMemoKey][]typ.Type)
		}
		i.specMemo[key] = rets
	}
	if len(rets) == 0 {
		return nil
	}
	final := make([]typ.Type, len(rets))
	for idx, ret := range rets {
		final[idx] = typ.Finalize(ret)
		if final[idx] == nil || typ.IsUnknown(final[idx]) {
			return nil
		}
	}
	return final
}

func overloadMembers(fn *typ.Function, cases []dispatchReturnCase) []typ.Type {
	var members []typ.Type
	for _, c := range cases {
		if c.site.paramIdx < 0 || c.site.paramIdx >= len(fn.Params) {
			continue
		}
		params := append([]typ.Param(nil), fn.Params...)
		params[c.site.paramIdx].Type = c.site.lit
		members = append(members, typjoin.WithReturns(fn, c.returns).WithParams(params))
	}
	return members
}

// collectLiteralDispatches finds distinct literals compared against
// parameters in HasType constraints of the function's flow inputs.
func collectLiteralDispatches(fnGraph *cfg.Graph, inputs *flow.Inputs) []dispatchSite {
	if fnGraph == nil || inputs == nil {
		return nil
	}
	symParam := make(map[cfg.SymbolID]int)
	for paramIdx, slot := range fnGraph.ParamSlotsReadOnly() {
		if slot.Symbol == 0 {
			continue
		}
		if !slot.HasSourceParam() {
			continue
		}
		symParam[slot.Symbol] = paramIdx
	}
	if len(symParam) == 0 {
		return nil
	}
	type siteKey struct {
		idx int
		lit uint64
	}
	seen := make(map[siteKey]bool)
	var out []dispatchSite
	for _, ec := range inputs.EdgeConditions {
		for _, c := range ec.Condition.AllConstraints() {
			ht, ok := c.(constraint.HasType)
			if !ok || len(ht.Path.Segments) != 0 {
				continue
			}
			idx, ok := symParam[ht.Path.Symbol]
			if !ok {
				continue
			}
			lit := literalForTypeKey(inputs, ht.Type)
			if lit == nil {
				continue
			}
			key := siteKey{idx: idx, lit: lit.Hash()}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, dispatchSite{paramIdx: idx, sym: ht.Path.Symbol, lit: lit})
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].paramIdx != out[b].paramIdx {
			return out[a].paramIdx < out[b].paramIdx
		}
		return out[a].lit.Hash() < out[b].lit.Hash()
	})
	return out
}

// literalForTypeKey resolves a hash type key to its literal value.
func literalForTypeKey(inputs *flow.Inputs, key narrow.TypeKey) *typ.Literal {
	if inputs == nil || inputs.TypeKeys == nil || key.Kind != narrow.TypeKeyHash {
		return nil
	}
	lit, ok := inputs.TypeKeys[key.Hash].(*typ.Literal)
	if !ok {
		return nil
	}
	return lit
}
