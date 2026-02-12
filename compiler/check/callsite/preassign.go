package callsite

import "github.com/wippyai/go-lua/compiler/cfg"

// PreAssignmentTargetsByCall indexes assignment target symbols by source call.
// For calls used as assignment RHS at point p (x = f(...)), the returned target
// set captures symbols that must be typed from predecessor state when computing
// argument evidence at that call site.
func PreAssignmentTargetsByCall(graph *cfg.Graph) map[*cfg.CallInfo]map[cfg.SymbolID]bool {
	if graph == nil {
		return nil
	}
	out := make(map[*cfg.CallInfo]map[cfg.SymbolID]bool)
	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if info == nil || len(info.Targets) == 0 || len(info.SourceCalls) == 0 {
			return
		}
		targets := make(map[cfg.SymbolID]bool, len(info.Targets))
		for _, target := range info.Targets {
			if target.Kind == cfg.TargetIdent && target.Symbol != 0 {
				targets[target.Symbol] = true
			}
		}
		if len(targets) == 0 {
			return
		}
		for _, call := range info.SourceCalls {
			if call == nil {
				continue
			}
			existing := out[call]
			if existing == nil {
				existing = make(map[cfg.SymbolID]bool, len(targets))
				out[call] = existing
			}
			for sym := range targets {
				existing[sym] = true
			}
		}
	})
	return out
}
