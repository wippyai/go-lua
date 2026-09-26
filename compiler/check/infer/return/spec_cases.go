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
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
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

// buildBodyDerivedReturnSpecs derives a return spec per eligible local
// function. Only single-return, non-recursive, unannotated functions with
// literal parameter dispatches qualify.
func (i *Inferencer) buildBodyDerivedReturnSpecs(
	run RunContext,
	sccs [][]cfg.SymbolID,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	summaries map[cfg.SymbolID][]typ.Type,
) map[cfg.SymbolID]*contract.Spec {
	skip := recursiveMembers(i, sccs)
	var out map[cfg.SymbolID]*contract.Spec
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
		if len(summary) != 1 {
			continue
		}
		spec := i.bodyReturnSpec(run, info, summaries, localFuncs, summary[0])
		if spec == nil {
			continue
		}
		if out == nil {
			out = make(map[cfg.SymbolID]*contract.Spec)
		}
		out[sym] = spec
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

// bodyReturnSpec builds the return spec for one function from its literal
// parameter dispatches.
func (i *Inferencer) bodyReturnSpec(
	run RunContext,
	info *returns.LocalFuncInfo,
	summaries map[cfg.SymbolID][]typ.Type,
	localFuncs map[cfg.SymbolID]*returns.LocalFuncInfo,
	def typ.Type,
) *contract.Spec {
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

	spec := contract.NewSpec()
	for _, site := range sites {
		t := i.specCaseType(ctx, untypedCapture, finalOverlay, info.Sym, site)
		if t == nil {
			continue
		}
		when := constraint.FromConstraints(constraint.HasType{
			Path: constraint.ParamPath(site.paramIdx),
			Type: narrow.HashTypeKey(site.lit.Hash()),
		})
		spec.WithReturnCase(when, t)
	}
	if len(spec.GetReturnCases()) == 0 {
		return nil
	}
	spec.WithDefaultReturn(typ.Finalize(def))
	return spec
}

// specCaseType reruns body return inference with one parameter fixed to one
// literal and reduces the result to a single case type.
func (i *Inferencer) specCaseType(
	ctx *returnInferenceContext,
	untypedCapture bool,
	finalOverlay map[cfg.SymbolID]typ.Type,
	sym cfg.SymbolID,
	site dispatchSite,
) typ.Type {
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
	if len(rets) != 1 {
		return nil
	}
	t := typ.Finalize(rets[0])
	if t == nil || typ.IsUnknown(t) {
		return nil
	}
	return t
}

// collectLiteralDispatches finds distinct literals compared against
// parameters in HasType constraints of the function's flow inputs.
func collectLiteralDispatches(fnGraph *cfg.Graph, inputs *flow.Inputs) []dispatchSite {
	if fnGraph == nil || inputs == nil {
		return nil
	}
	symParam := make(map[cfg.SymbolID]int)
	for _, slot := range fnGraph.ParamSlotsReadOnly() {
		if slot.Symbol == 0 {
			continue
		}
		idx, ok := slot.SourceParamIndex()
		if !ok {
			continue
		}
		symParam[slot.Symbol] = idx
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

// attachBodyReturnSpec attaches derived cases to a function type without
// disturbing an existing contract specification.
func attachBodyReturnSpec(fnType *typ.Function, spec *contract.Spec) {
	if fnType == nil || spec == nil || len(spec.GetReturnCases()) == 0 {
		return
	}
	if existing, ok := fnType.Spec.(*contract.Spec); ok && existing != nil {
		for _, rc := range spec.GetReturnCases() {
			existing.WithReturnCase(rc.When, rc.Type)
		}
		if existing.GetReturnDefault() == nil {
			existing.WithDefaultReturn(spec.GetReturnDefault())
		}
		return
	}
	if fnType.Spec == nil {
		fnType.Spec = spec
	}
}
