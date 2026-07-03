package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func runtimeTypeName(name string) (typ.Type, bool) {
	switch name {
	case "nil":
		return typ.Nil, true
	case "boolean":
		return typ.Boolean, true
	case "number":
		return typ.Number, true
	case "string":
		return typ.String, true
	case "function":
		return typ.Func().Build(), true
	default:
		return nil, false
	}
}

func runtimeTypeGuardProves(name string, want typ.Type) bool {
	if got, ok := runtimeTypeName(name); ok && subtype.IsSubtype(got, want) {
		return true
	}
	return name == "table" && runtimeTableGuardProves(want)
}

func runtimeTableGuardProves(want typ.Type) bool {
	if want == nil {
		return false
	}
	if typetable.IsBuiltinTopMarker(want) {
		return true
	}
	switch t := unwrap.Annotated(want).(type) {
	case *typ.Alias:
		return runtimeTableGuardProves(t.UnaliasedTarget())
	case *typ.Optional:
		return runtimeTableGuardProves(t.Inner)
	case *typ.Map:
		return runtimeTableMapGuardProves(t.Key, t.Value)
	case *typ.ReadonlyMap:
		return runtimeTableMapGuardProves(t.Key, t.Value)
	case *typ.Record:
		if len(t.Fields) != 0 {
			return false
		}
		if t.Open && !t.HasMapComponent() {
			return true
		}
		return t.HasMapComponent() && runtimeTableMapGuardProves(t.MapKey, t.MapValue)
	case *typ.Union:
		for _, member := range t.Members {
			if runtimeTableGuardProves(member) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func runtimeTableMapGuardProves(key, value typ.Type) bool {
	if !runtimeTableMapKeyAdmitted(key) || !topLikeType(value) {
		return false
	}
	return true
}

func runtimeTableMapKeyAdmitted(key typ.Type) bool {
	return topLikeType(key) ||
		typ.TypeEquals(key, typ.String) ||
		typ.TypeEquals(key, typ.Number) ||
		typ.TypeEquals(key, typ.Integer)
}

func dominantRuntimeTypeGuard(result *body.Result, point cfg.Point, p path.Path, want typ.Type) bool {
	graph := result.Graph()
	if graph == nil || point == 0 || p.IsEmpty() {
		return false
	}
	dom := dominance.ComputeImmediateDominatorInfo(graph)
	for _, branch := range cfg.RPOReadOnly(graph) {
		if !dom.StrictlyDominates(branch, point) {
			continue
		}
		fact, ok := result.BranchCondition(branch)
		if !ok || !fact.Check.Path.Equal(p) {
			continue
		}
		rejectCond, ok := runtimeTypeGuardRejectCond(fact.Check.Kind)
		if !ok {
			continue
		}
		if !runtimeTypeGuardProves(fact.Check.TypeName, want) {
			continue
		}
		for _, succ := range cfg.SuccessorsReadOnly(graph, branch) {
			cond, ok := graph.EdgeCond(branch, succ)
			if !ok || cond != rejectCond {
				continue
			}
			if !cfg.PointCanReach(graph, succ, point) {
				return true
			}
		}
	}
	return false
}

func runtimeTypeGuardRejectCond(kind branchcond.CheckKind) (bool, bool) {
	switch kind {
	case branchcond.CheckTypeEqual:
		return false, true
	case branchcond.CheckTypeNot:
		return true, true
	default:
		return false, false
	}
}
