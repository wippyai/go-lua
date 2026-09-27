package returns

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

// DefinitionView derives symbol facts by folding every literal bound to each symbol.
func DefinitionView(graph *cfg.Graph, callables api.Callables) map[cfg.SymbolID]api.FunctionFact {
	if graph == nil || graph.Bindings() == nil || len(callables) == 0 {
		return nil
	}
	points := make(map[*ast.FunctionExpr]cfg.Point)
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		for _, source := range info.Sources {
			if fn, ok := source.(*ast.FunctionExpr); ok {
				points[fn] = p
			}
		}
	})
	graph.EachFuncDef(func(p cfg.Point, info *cfg.FuncDefInfo) {
		if info.FuncExpr != nil {
			points[info.FuncExpr] = p
		}
	})
	literals := make([]*ast.FunctionExpr, 0, len(callables))
	for fn := range callables {
		if fn != nil {
			literals = append(literals, fn)
		}
	}
	sort.Slice(literals, func(i, j int) bool {
		a, b := literals[i], literals[j]
		ap, bp := points[a], points[b]
		if ap != bp {
			return ap < bp
		}
		if a.Line() != b.Line() {
			return a.Line() < b.Line()
		}
		return a.Column() < b.Column()
	})
	out := make(map[cfg.SymbolID]api.FunctionFact)
	for _, fn := range literals {
		sym, ok := graph.Bindings().FuncLitSymbol(fn)
		if !ok || sym == 0 {
			continue
		}
		fact := callables[fn]
		existing := out[sym]
		merged := ReconcileFunctionFact(ReconcileFunctionFactInput{
			ExistingSummary: existing.Summary, ExistingNarrow: existing.Narrow, ExistingFunc: existing.Func,
			CandidateSummary: fact.Summary, CandidateNarrow: fact.Narrow, CandidateFunc: fact.Func,
		})
		out[sym] = api.FunctionFact{Summary: merged.Summary, Narrow: merged.Narrow, Func: merged.Func}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func SummaryViewFromFacts(graph *cfg.Graph, facts api.Facts) api.ReturnSummaries {
	var out api.ReturnSummaries
	for sym, fact := range DefinitionView(graph, facts.Callables) {
		if len(fact.Summary) > 0 {
			if out == nil {
				out = make(api.ReturnSummaries)
			}
			out[sym] = fact.Summary
		}
	}
	return out
}

func NarrowViewFromFacts(graph *cfg.Graph, facts api.Facts) api.NarrowReturnSummaries {
	var out api.NarrowReturnSummaries
	for sym, fact := range DefinitionView(graph, facts.Callables) {
		if len(fact.Narrow) > 0 {
			if out == nil {
				out = make(api.NarrowReturnSummaries)
			}
			out[sym] = fact.Narrow
		}
	}
	return out
}

func FuncTypeViewFromFacts(graph *cfg.Graph, facts api.Facts) api.FuncTypes {
	var out api.FuncTypes
	for sym, fact := range DefinitionView(graph, facts.Callables) {
		if fact.Func != nil {
			if out == nil {
				out = make(map[cfg.SymbolID]typ.Type)
			}
			out[sym] = fact.Func
		}
	}
	return out
}
