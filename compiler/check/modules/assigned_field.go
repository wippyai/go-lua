package modules

import "github.com/wippyai/go-lua/compiler/cfg"

// AssignedModuleField reports whether a graph may replace a module's direct
// field. Once replaced, a body-backed summary for that field is not reliable.
func AssignedModuleField(graph *cfg.Graph, symbol cfg.SymbolID, field string) bool {
	if graph == nil || symbol == 0 || field == "" {
		return false
	}
	replaced := false
	graph.EachAssign(func(_ cfg.Point, assignment *cfg.AssignInfo) {
		if assignment == nil || replaced {
			return
		}
		for _, target := range assignment.Targets {
			if target.Kind == cfg.TargetField && target.BaseSymbol == symbol &&
				len(target.FieldPath) == 1 && target.FieldPath[0] == field {
				replaced = true
				return
			}
		}
	})
	return replaced
}
