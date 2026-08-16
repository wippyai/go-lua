package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/domain/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

func RuntimeTypeGuardProves(name string, want typ.Type) bool {
	if got, ok := typ.BuiltinPrimitiveType(name); ok && subtype.IsSubtype(got, want) {
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
	if !runtimeTableMapKeyAdmitted(key) || !typ.IsTopLike(value) {
		return false
	}
	return true
}

func runtimeTableMapKeyAdmitted(key typ.Type) bool {
	return typ.IsTopLike(key) ||
		typ.TypeEquals(key, typ.String) ||
		typ.TypeEquals(key, typ.Number) ||
		typ.TypeEquals(key, typ.Integer)
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
		check, ok := r.BranchConditionCheck(branch)
		if !ok || !check.Path.Equal(p) {
			continue
		}
		rejectCond, ok := runtimeTypeGuardRejectCond(check.Kind)
		if !ok {
			continue
		}
		if !RuntimeTypeGuardProves(check.TypeName, want) {
			continue
		}
		successors := cfg.SuccessorsReadOnly(graph, branch)
		conditions := cfg.SuccessorConditionsReadOnly(graph, branch)
		if len(conditions) != len(successors) {
			continue
		}
		for index, succ := range successors {
			if conditions[index] != rejectCond {
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
