package returns

import (
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
)

// localCallInstance binds a returned closure to one factory invocation. Each
// captured symbol denotes a cell; returned cells are mapped to the matching
// local produced by that same invocation.
type localCallInstance struct {
	function cfg.SymbolID
	captures map[cfg.SymbolID]cfg.SymbolID
}

func (s *StoreFieldWriteSource) resolvedCall(graph *cfg.Graph, bindings *bind.BindingTable, info *cfg.CallInfo) localCallInstance {
	if s == nil || s.Store == nil || graph == nil || info == nil {
		return localCallInstance{}
	}
	candidates := checkcallsite.CallableCalleeSymbolCandidates(info, graph, bindings, bindings)
	var found localCallInstance
	for _, sym := range candidates {
		if s.Store.FunctionRefBySym(sym) != nil {
			if found.function != 0 && found.function != sym {
				return localCallInstance{}
			}
			found = localCallInstance{function: sym}
			continue
		}
		if instance := s.factoryInstances(graph, bindings)[sym]; instance.function != 0 {
			if found.function != 0 && found.function != instance.function {
				return localCallInstance{}
			}
			found = instance
		}
	}
	return found
}

func (s *StoreFieldWriteSource) factoryInstances(graph *cfg.Graph, bindings *bind.BindingTable) map[cfg.SymbolID]localCallInstance {
	if s.instances == nil {
		s.instances = make(map[uint64]map[cfg.SymbolID]localCallInstance)
	}
	if existing, ok := s.instances[graph.ID()]; ok {
		return existing
	}
	result := make(map[cfg.SymbolID]localCallInstance)
	s.instances[graph.ID()] = result
	if bindings == nil {
		return result
	}
	assignments := make(map[cfg.SymbolID]int)
	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		for _, target := range info.Targets {
			if target.Kind == cfg.TargetIdent && target.Symbol != 0 {
				assignments[target.Symbol]++
			}
		}
	})
	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if info == nil || len(info.SourceCalls) != 1 || info.SourceCalls[0] == nil {
			return
		}
		factoryCall := info.SourceCalls[0]
		candidates := checkcallsite.CallableCalleeSymbolCandidates(factoryCall, graph, bindings, bindings)
		if len(candidates) != 1 {
			return
		}
		factory := s.Store.FunctionRefBySym(candidates[0])
		if factory == nil {
			return
		}
		body := s.Store.Graphs()[factory.GraphID]
		if body == nil {
			return
		}
		var returned []cfg.SymbolID
		returns := 0
		body.EachReturn(func(_ cfg.Point, ret *cfg.ReturnInfo) {
			returns++
			if ret != nil {
				returned = ret.Symbols
			}
		})
		if returns != 1 || len(returned) < len(info.Targets) {
			return
		}
		captures := make(map[cfg.SymbolID]cfg.SymbolID)
		for idx, target := range info.Targets {
			if target.Kind == cfg.TargetIdent && target.Symbol != 0 && returned[idx] != 0 && assignments[target.Symbol] == 1 {
				captures[returned[idx]] = target.Symbol
			}
		}
		for idx, target := range info.Targets {
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 || assignments[target.Symbol] != 1 {
				continue
			}
			closure := s.Store.FunctionRefBySym(returned[idx])
			if closure == nil || closure.ParentGraphID != body.ID() {
				continue
			}
			result[target.Symbol] = localCallInstance{function: returned[idx], captures: captures}
		}
	})
	return result
}
