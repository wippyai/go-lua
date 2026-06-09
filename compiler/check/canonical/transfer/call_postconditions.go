package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

type assignCallPostconditionEffects struct {
	relations           []flow.RelationEffect
	boundaryFacts       []boundaryFactPostcondition
	returnStaticMembers []returnStaticMemberPostcondition
	numericOps          []flow.NumericOp
}

type boundaryFactPostcondition struct {
	call    *ast.FuncCallExpr
	point   cfg.Point
	facts   flow.BoundaryFacts
	returns map[int]constraint.Path
	plan    flow.BoundaryFactReplayPlan
}

type returnStaticMemberPostcondition struct {
	target constraint.Path
	facts  flow.StaticMemberFacts
}

// buildAssignCallPostconditions rebases callee return-relation predicates into
// caller facts before assignment targets are overwritten. `ReturnRelations` is
// the normalized call-postcondition carrier: every atom is interpreted here by
// mapping callee return slots to assignment targets and callee runtime parameters
// to call-site runtime arguments, then lowering through the canonical reducers.
// Boundary-relative key and map postconditions are carried separately by
// `BoundaryFacts` so flow owns their path rebasing and point-state replay.
//
// Representative case: `keys(data)` proves `len(return[0]) >= len(param[0])`.
// If the caller already knows `len(data) >= 1`, this materializes `len(ks) >= 1`
// so ordinary index-read refinement can prove `ks[1]` present. If `data` is a
// caller parameter, the same atom also seeds a point-local length-param relation
// so wrapper functions can re-export the proof. No function-specific branch
// belongs here; new postcondition atoms extend this materializer.
func (t *Transfer) buildAssignCallPostconditions(
	out *flow.PointState,
	p cfg.Point,
	info *cfg.AssignInfo,
	callApps assignCallApplications,
) assignCallPostconditionEffects {
	var effects assignCallPostconditionEffects
	if out == nil || info == nil {
		return effects
	}
	info.EachSourceCall(func(_ int, callInfo *cfg.CallInfo) {
		if callInfo == nil || callInfo.Call == nil {
			return
		}
		app, ok := callApps.byCall[callInfo.Call]
		if !ok {
			return
		}
		boundary := app.Result.Boundary
		t.appendSiblingNilPostconditions(info, callInfo, boundary.ReturnRelations, &effects)
		t.appendGuardedTypePostconditions(info, callInfo, boundary.ReturnRelations, &effects)
		t.appendBoundaryFactPostconditions(out, p, info, callInfo, boundary.BoundaryFacts, &effects)
		t.appendReturnStaticMemberPostconditions(info, callInfo, boundary.ReturnStaticMembers, &effects)
	})
	return effects
}

func (t *Transfer) applyAssignCallPostconditions(out *flow.PointState, effects assignCallPostconditionEffects) {
	if out == nil {
		return
	}
	for _, rel := range effects.relations {
		flow.ApplyRelationEffect(out, rel)
	}
	for _, effect := range effects.boundaryFacts {
		t.applyBoundaryFactsWithPlan(out, effect.point, effect.call, effect.facts, effect.returns, effect.plan)
	}
	for _, effect := range effects.returnStaticMembers {
		flow.ApplyStaticMemberFacts(out, effect.facts)
	}
	if len(effects.numericOps) > 0 {
		flow.ApplyNumericEffect(out, flow.NumericEffect{Ops: effects.numericOps})
	}
}

func (t *Transfer) appendSiblingNilPostconditions(
	info *cfg.AssignInfo,
	callInfo *cfg.CallInfo,
	rels flow.ReturnRelations,
	effects *assignCallPostconditionEffects,
) {
	for _, corr := range rels.ErrorReturns() {
		valTarget, ok := assignmentTargetForReturn(info, callInfo, corr.ValueIndex)
		if !ok || valTarget.Kind != cfg.TargetIdent || valTarget.Symbol == 0 {
			continue
		}
		errTarget, ok := assignmentTargetForReturn(info, callInfo, corr.ErrorIndex)
		if !ok || errTarget.Kind != cfg.TargetIdent || errTarget.Symbol == 0 {
			continue
		}
		effects.relations = append(effects.relations, flow.RelationEffect{
			Kind:      flow.RelationSeedSiblingNil,
			ErrSym:    errTarget.Symbol,
			ValueSyms: []cfg.SymbolID{valTarget.Symbol},
		})
	}
}

func (t *Transfer) appendGuardedTypePostconditions(
	info *cfg.AssignInfo,
	callInfo *cfg.CallInfo,
	rels flow.ReturnRelations,
	effects *assignCallPostconditionEffects,
) {
	for _, rel := range rels.GuardedTypes() {
		guardTarget, ok := assignmentTargetForReturn(info, callInfo, rel.GuardIndex)
		if !ok || guardTarget.Kind != cfg.TargetIdent || guardTarget.Symbol == 0 {
			continue
		}
		valueTarget, ok := assignmentTargetForReturn(info, callInfo, rel.TargetIndex)
		if !ok || valueTarget.Kind != cfg.TargetIdent || valueTarget.Symbol == 0 || rel.TargetType == nil {
			continue
		}
		effects.relations = append(effects.relations, flow.RelationEffect{
			Kind:          flow.RelationSeedGuardedType,
			GuardSym:      guardTarget.Symbol,
			TargetSym:     valueTarget.Symbol,
			GuardOnTruthy: rel.GuardOnTruthy,
			TargetType:    rel.TargetType,
		})
	}
}

func (t *Transfer) appendBoundaryFactPostconditions(
	out *flow.PointState,
	p cfg.Point,
	info *cfg.AssignInfo,
	callInfo *cfg.CallInfo,
	facts flow.BoundaryFacts,
	effects *assignCallPostconditionEffects,
) {
	if callInfo == nil || callInfo.Call == nil {
		return
	}
	if !facts.HasProof() {
		return
	}
	returns := t.boundaryReturnPaths(info, callInfo, facts)
	roots := t.callBoundaryLocalRootsAt(out, p, callInfo.Call, returns)
	effects.boundaryFacts = append(effects.boundaryFacts, boundaryFactPostcondition{
		call:    callInfo.Call,
		point:   p,
		facts:   facts,
		returns: returns,
		plan:    flow.PrepareBoundaryFactReplay(*out, facts, roots),
	})
}

func (t *Transfer) appendReturnStaticMemberPostconditions(
	info *cfg.AssignInfo,
	callInfo *cfg.CallInfo,
	members []flow.StaticMemberFacts,
	effects *assignCallPostconditionEffects,
) {
	if callInfo == nil || callInfo.Call == nil || effects == nil || len(members) == 0 {
		return
	}
	for i, facts := range members {
		if !facts.HasProof() {
			continue
		}
		target, ok := assignmentTargetForReturn(info, callInfo, i)
		if !ok {
			continue
		}
		targetPath, ok := t.staticPathOfAssignTarget(target)
		if !ok || targetPath.IsEmpty() {
			continue
		}
		targetAddr, ok := flow.StableAddressOfPath(targetPath)
		if !ok {
			continue
		}
		slotAddr, ok := flow.StableAddressOfPath(constraint.NewPlaceholder(i))
		if !ok {
			continue
		}
		rebased := flow.RebaseStaticMemberFactsUnder(facts, slotAddr, targetAddr)
		if !rebased.HasProof() {
			continue
		}
		effects.returnStaticMembers = append(effects.returnStaticMembers, returnStaticMemberPostcondition{
			target: targetPath,
			facts:  rebased,
		})
	}
}

func (t *Transfer) boundaryReturnPaths(
	info *cfg.AssignInfo,
	callInfo *cfg.CallInfo,
	facts flow.BoundaryFacts,
) map[int]constraint.Path {
	_, buckets := facts.PartitionByReturnIndices()
	if len(buckets) == 0 {
		return nil
	}
	out := make(map[int]constraint.Path)
	for _, bucket := range buckets {
		for _, index := range bucket.Indices() {
			t.collectBoundaryReturnIndex(info, callInfo, index, out)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (t *Transfer) collectBoundaryReturnIndex(
	info *cfg.AssignInfo,
	callInfo *cfg.CallInfo,
	index int,
	out map[int]constraint.Path,
) {
	if _, ok := out[index]; ok {
		return
	}
	target, ok := assignmentTargetForReturn(info, callInfo, index)
	if !ok {
		return
	}
	targetPath, ok := t.staticPathOfAssignTarget(target)
	if !ok || targetPath.IsEmpty() {
		return
	}
	out[index] = targetPath
}

func assignmentTargetForReturn(info *cfg.AssignInfo, callInfo *cfg.CallInfo, retIndex int) (cfg.AssignTarget, bool) {
	if info == nil || callInfo == nil || retIndex < 0 {
		return cfg.AssignTarget{}, false
	}
	for i := range info.Targets {
		call, idx := info.CallForTarget(i)
		if call == callInfo && idx == retIndex {
			return info.Targets[i], true
		}
	}
	return cfg.AssignTarget{}, false
}

func (t *Transfer) callerParamIndexForPath(path constraint.Path) (int, bool) {
	if path.Symbol == 0 || len(path.Segments) != 0 {
		return 0, false
	}
	param, ok := t.params.Lookup(path.Symbol)
	return param.Index, ok
}
