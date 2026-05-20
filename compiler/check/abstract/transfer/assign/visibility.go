package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

type symbolVersionKey struct {
	sym cfg.SymbolID
	id  int
}

func paramSymbolSet(graph *cfg.Graph) map[cfg.SymbolID]bool {
	if graph == nil {
		return nil
	}
	params := graph.ParamSymbols()
	if len(params) == 0 {
		return nil
	}
	out := make(map[cfg.SymbolID]bool, len(params))
	for _, sym := range params {
		if sym != 0 {
			out[sym] = true
		}
	}
	return out
}

func collectValueDefinitionVersions(
	graph *cfg.Graph,
	assignments []api.AssignmentEvidence,
	functions []api.FunctionDefinitionEvidence,
) map[symbolVersionKey]struct{} {
	if graph == nil {
		return nil
	}
	out := make(map[symbolVersionKey]struct{})
	for _, assign := range assignments {
		p := assign.Point
		info := assign.Info
		if info == nil {
			continue
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
			if source == nil && info.NumericFor == nil && len(info.IterExprs) == 0 {
				return
			}
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				return
			}
			if ver := graph.VisibleVersion(p, target.Symbol); ver.Symbol != 0 && ver.ID != 0 {
				out[symbolVersionKey{sym: target.Symbol, id: ver.ID}] = struct{}{}
			}
		})
	}
	for _, def := range functions {
		p := def.Nested.Point
		info := def.FuncDef
		if info == nil || info.Symbol == 0 || info.FuncExpr == nil {
			continue
		}
		if ver := graph.VisibleVersion(p, info.Symbol); ver.Symbol != 0 && ver.ID != 0 {
			out[symbolVersionKey{sym: info.Symbol, id: ver.ID}] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func overlayTypeVisibleAt(
	graph *cfg.Graph,
	valueDefs map[symbolVersionKey]struct{},
	paramSet map[cfg.SymbolID]bool,
	sym cfg.SymbolID,
	p cfg.Point,
) bool {
	if sym == 0 {
		return false
	}
	if graph == nil {
		return true
	}
	if paramSet[sym] {
		return true
	}
	ver := graph.VisibleVersion(p, sym)
	if ver.Symbol == 0 || ver.ID == 0 {
		return false
	}
	_, ok := valueDefs[symbolVersionKey{sym: sym, id: ver.ID}]
	return ok
}

func visibleInferredTypeAt(
	inferred api.SpecTypes,
	graph *cfg.Graph,
	valueDefs map[symbolVersionKey]struct{},
	paramSet map[cfg.SymbolID]bool,
	sym cfg.SymbolID,
	p cfg.Point,
) (typ.Type, bool) {
	t, ok := inferred[sym]
	if !ok {
		return nil, false
	}
	if !overlayTypeVisibleAt(graph, valueDefs, paramSet, sym, p) {
		return nil, false
	}
	return t, true
}

func mergeVisibleInferredTypes(
	out api.SpecTypes,
	inferred api.SpecTypes,
	graph *cfg.Graph,
	valueDefs map[symbolVersionKey]struct{},
	paramSet map[cfg.SymbolID]bool,
	p cfg.Point,
) api.SpecTypes {
	if len(inferred) == 0 {
		return out
	}
	for sym, t := range inferred {
		if !overlayTypeVisibleAt(graph, valueDefs, paramSet, sym, p) {
			continue
		}
		if out == nil {
			out = make(api.SpecTypes, len(inferred))
		}
		out[sym] = t
	}
	return out
}

func inferenceOverlayAtPoint(
	graph *cfg.Graph,
	p cfg.Point,
	inferred api.SpecTypes,
	seedTypes api.SpecTypes,
	funcSigTypes map[cfg.SymbolID]typ.Type,
	valueDefs map[symbolVersionKey]struct{},
	paramSet map[cfg.SymbolID]bool,
) api.SpecTypes {
	var out api.SpecTypes
	out = mergeSpecTypesInto(out, seedTypes)
	for sym, t := range funcSigTypes {
		if !overlayTypeVisibleAt(graph, valueDefs, paramSet, sym, p) {
			continue
		}
		if out == nil {
			out = make(api.SpecTypes, len(funcSigTypes))
		}
		out[sym] = t
	}
	out = mergeVisibleInferredTypes(out, inferred, graph, valueDefs, paramSet, p)
	return out
}
