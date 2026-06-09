package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	domainpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

func (t *Transfer) applyBoundaryFacts(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	facts flow.BoundaryFacts,
	returns map[int]constraint.Path,
) bool {
	if out == nil {
		return false
	}
	roots := t.callBoundaryLocalRootsForState(out, call, returns)
	plan := flow.PrepareBoundaryFactReplay(*out, facts, roots)
	return t.applyBoundaryFactsWithPlan(out, cfg.Point(0), call, facts, returns, plan)
}

func (t *Transfer) applyBoundaryFactsWithPlan(
	out *flow.PointState,
	p cfg.Point,
	call *ast.FuncCallExpr,
	facts flow.BoundaryFacts,
	returns map[int]constraint.Path,
	plan flow.BoundaryFactReplayPlan,
) bool {
	if out == nil || call == nil {
		return false
	}
	roots := t.callBoundaryLocalRootsAt(out, p, call, returns)
	app, changed := flow.ApplyBoundaryFactsWithReplay(out, facts, roots, plan)
	t.applyBoundaryFactApplication(out, p, app)
	return changed || boundaryFactApplicationHasEffects(app)
}

func (t *Transfer) applyBoundaryFactApplication(out *flow.PointState, p cfg.Point, app flow.BoundaryFactApplication) {
	for _, result := range app.KeyProvenance {
		t.applyKeyProvenanceResult(out, result)
	}
	for _, rel := range app.LengthRelations {
		paramIndex, ok := t.callerParamIndexForPath(rel.Source)
		if !ok {
			continue
		}
		targetLocalPath := domainpath.WithVersion(rel.Target, t.in.Graph, p)
		targetLocal, targetOK := flow.LocalAddressOfPath(targetLocalPath)
		if !targetOK {
			continue
		}
		if effect, ok := flow.RelationTargetLengthParamLocalEffect(targetLocal, paramIndex); ok {
			flow.ApplyRelationEffect(out, effect)
		}
	}
}

func boundaryFactApplicationHasEffects(app flow.BoundaryFactApplication) bool {
	return len(app.KeyProvenance) > 0 || len(app.LengthRelations) > 0
}

func (t *Transfer) callBoundaryLocalRootsForState(out *flow.PointState, call *ast.FuncCallExpr, returns map[int]constraint.Path) flow.BoundaryLocalRoots {
	return t.callBoundaryLocalRootsAt(out, cfg.Point(0), call, returns)
}

func (t *Transfer) callBoundaryLocalRootsAt(out *flow.PointState, p cfg.Point, call *ast.FuncCallExpr, returns map[int]constraint.Path) flow.BoundaryLocalRoots {
	if t == nil || call == nil {
		return flow.NewBoundaryLocalRoots(nil, returns)
	}
	count := callsite.RuntimeArgExprCount(call)
	params := make(map[int]constraint.Path, count)
	for idx := 0; idx < count; idx++ {
		arg := callsite.RuntimeArgExprAt(call, idx)
		argPath, ok := t.callBoundaryArgPath(out, p, arg)
		if ok && !argPath.IsEmpty() {
			params[idx] = argPath
		}
	}
	return flow.NewBoundaryLocalRoots(params, returns)
}

func (t *Transfer) callBoundaryArgPath(out *flow.PointState, p cfg.Point, arg ast.Expr) (constraint.Path, bool) {
	if t == nil || arg == nil {
		return constraint.Path{}, false
	}
	if out != nil {
		if place, ok := t.placeOfExprAt(out, p, arg, nil); ok {
			if path, pathOK := place.StaticPath(); pathOK && !path.IsEmpty() {
				return path, true
			}
		}
	}
	argPath, ok := t.staticPathOfExpr(arg)
	if !ok || argPath.IsEmpty() {
		return constraint.Path{}, false
	}
	if p != 0 && t.in.Graph != nil {
		argPath = domainpath.WithVersion(argPath, t.in.Graph, p)
	}
	return argPath, true
}
