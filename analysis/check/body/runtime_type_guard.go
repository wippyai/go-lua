package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func RuntimeTypeGuardProves(name string, want typ.Type) bool {
	if got, ok := runtimeTypeName(name); ok && subtype.IsSubtype(got, want) {
		return true
	}
	return name == "table" && runtimeTableGuardProves(want)
}

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

func topLikeType(t typ.Type) bool {
	return typ.IsAny(t) || typ.IsUnknown(t)
}

// DominatingRuntimeTypeGuardProves reports whether a dominating runtime
// type-check branch proves p has type want at point, and the rejecting branch
// cannot reach point.
func (r *Result) DominatingRuntimeTypeGuardProves(point cfg.Point, p pathdom.Path, want typ.Type) bool {
	graph := r.Graph()
	if graph == nil || point == 0 || p.IsEmpty() {
		return false
	}
	for _, branch := range cfg.RPOReadOnly(graph) {
		if branch == point || !r.PointDominates(branch, point) {
			continue
		}
		fact, ok := r.BranchCondition(branch)
		if !ok || !fact.Check.Path.Equal(p) {
			continue
		}
		rejectCond, ok := runtimeTypeGuardRejectCond(fact.Check.Kind)
		if !ok {
			continue
		}
		if !RuntimeTypeGuardProves(fact.Check.TypeName, want) {
			continue
		}
		for _, succ := range cfg.SuccessorsReadOnly(graph, branch) {
			cond, ok := graph.EdgeCond(branch, succ)
			if !ok || cond != rejectCond {
				continue
			}
			if !r.PointCanReach(succ, point) {
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
