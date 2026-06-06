package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

type assignCallPostconditionEffects struct {
	relations     []flow.RelationEffect
	keyProvenance []flow.KeyProvenancePathProof
	boundaryFacts []boundaryFactPostcondition
	numericOps    []flow.NumericOp
}

type boundaryFactPostcondition struct {
	call        *ast.FuncCallExpr
	facts       flow.BoundaryFacts
	returns     map[int]constraint.Path
	appendPlans []boundaryAppendKeyPlan
}

// buildAssignCallPostconditions rebases callee return-relation predicates into
// caller facts before assignment targets are overwritten. `ReturnRelations` is
// the normalized call-postcondition carrier: every atom is interpreted here by
// mapping callee return slots to assignment targets and callee runtime parameters
// to call-site runtime arguments, then lowering through the canonical reducers.
//
// Representative case: `keys(data)` proves `len(return[0]) >= len(param[0])`.
// If the caller already knows `len(data) >= 1`, this materializes `len(ks) >= 1`
// so ordinary index-read refinement can prove `ks[1]` present. If `data` is a
// caller parameter, the same atom also seeds a point-local length-param relation
// so wrapper functions can re-export the proof. No function-specific branch
// belongs here; new postcondition atoms extend this materializer.
func (t *Transfer) buildAssignCallPostconditions(
	out *flow.PointState,
	info *cfg.AssignInfo,
	demand func(int, paramevidence.ParamContract),
) assignCallPostconditionEffects {
	var effects assignCallPostconditionEffects
	if out == nil || info == nil {
		return effects
	}
	info.EachSourceCall(func(_ int, callInfo *cfg.CallInfo) {
		if callInfo == nil || callInfo.Call == nil {
			return
		}
		rels := t.callReturnRelations(out, callInfo.Call, demand)
		t.appendSiblingNilPostconditions(info, callInfo, rels, &effects)
		t.appendGuardedTypePostconditions(info, callInfo, rels, &effects)
		t.appendLengthParamPostconditions(out, info, callInfo, rels, &effects)
		t.appendReturnKeyParamPostconditions(info, callInfo, rels, &effects)
		t.appendBoundaryFactPostconditions(out, info, callInfo, demand, &effects)
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
	for _, effect := range effects.keyProvenance {
		t.applyKeyProvenancePathProof(out, effect)
	}
	for _, effect := range effects.boundaryFacts {
		t.applyBoundaryFactsWithAppendPlans(out, effect.call, effect.facts, effect.returns, effect.appendPlans)
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

func (t *Transfer) appendReturnKeyParamPostconditions(
	info *cfg.AssignInfo,
	callInfo *cfg.CallInfo,
	rels flow.ReturnRelations,
	effects *assignCallPostconditionEffects,
) {
	for _, rel := range rels.KeyParams() {
		target, ok := assignmentTargetForReturn(info, callInfo, rel.ReturnIndex)
		if !ok {
			continue
		}
		keyPath, ok := t.staticPathOfAssignTarget(target)
		if !ok || keyPath.IsEmpty() {
			continue
		}
		arg := callsite.RuntimeArgAt(callInfo, rel.ParamIndex)
		if arg == nil {
			continue
		}
		tablePath, ok := t.staticPathOfExpr(arg)
		if !ok || tablePath.IsEmpty() {
			continue
		}
		for _, seg := range rel.ParamSegments {
			tablePath = tablePath.Append(seg)
		}
		effects.keyProvenance = append(effects.keyProvenance, flow.KeyProvenancePathProof{
			Kind:      flow.KeyProvenanceDynamicIndexWrite,
			TablePath: tablePath,
			KeyPath:   keyPath,
		})
	}
}

func (t *Transfer) appendBoundaryFactPostconditions(
	out *flow.PointState,
	info *cfg.AssignInfo,
	callInfo *cfg.CallInfo,
	demand func(int, paramevidence.ParamContract),
	effects *assignCallPostconditionEffects,
) {
	if callInfo == nil || callInfo.Call == nil {
		return
	}
	facts := t.callBoundaryFacts(out, callInfo.Call, demand)
	if !facts.HasProof() {
		return
	}
	returns := make(map[int]constraint.Path)
	for _, fact := range facts.KeyPresence() {
		t.collectBoundaryReturnPath(info, callInfo, fact.Table, returns)
		t.collectBoundaryReturnPath(info, callInfo, fact.Key, returns)
	}
	for _, fact := range facts.KeyArrays() {
		t.collectBoundaryReturnPath(info, callInfo, fact.Array, returns)
		t.collectBoundaryReturnPath(info, callInfo, fact.Table, returns)
	}
	for _, fact := range facts.KeyArrayValues() {
		t.collectBoundaryReturnPath(info, callInfo, fact.Array, returns)
		t.collectBoundaryReturnPath(info, callInfo, fact.Table, returns)
	}
	for _, fact := range facts.AppendKeys() {
		t.collectBoundaryReturnPath(info, callInfo, fact.Array, returns)
		t.collectBoundaryReturnPath(info, callInfo, fact.Key, returns)
		if fact.HasTable {
			t.collectBoundaryReturnPath(info, callInfo, fact.Table, returns)
		}
	}
	for _, fact := range facts.AppendElementFieldOrigins() {
		t.collectBoundaryReturnPath(info, callInfo, fact.Array, returns)
		t.collectBoundaryReturnPath(info, callInfo, fact.Source, returns)
	}
	for _, fact := range facts.LengthLowerBounds() {
		t.collectBoundaryReturnPath(info, callInfo, fact.Target, returns)
	}
	for _, fact := range facts.IndexWrites() {
		t.collectBoundaryReturnPath(info, callInfo, fact.Table, returns)
		t.collectBoundaryReturnPath(info, callInfo, fact.Key, returns)
	}
	effects.boundaryFacts = append(effects.boundaryFacts, boundaryFactPostcondition{
		call:        callInfo.Call,
		facts:       facts,
		returns:     returns,
		appendPlans: t.boundaryAppendKeyPlans(out, callInfo.Call, facts, returns),
	})
}

func (t *Transfer) collectBoundaryReturnPath(
	info *cfg.AssignInfo,
	callInfo *cfg.CallInfo,
	path flow.BoundaryPath,
	out map[int]constraint.Path,
) {
	if path.Kind != flow.BoundaryPathReturn {
		return
	}
	if _, ok := out[path.Index]; ok {
		return
	}
	target, ok := assignmentTargetForReturn(info, callInfo, path.Index)
	if !ok {
		return
	}
	targetPath, ok := t.staticPathOfAssignTarget(target)
	if !ok || targetPath.IsEmpty() {
		return
	}
	out[path.Index] = targetPath
}

func (t *Transfer) appendLengthParamPostconditions(
	out *flow.PointState,
	info *cfg.AssignInfo,
	callInfo *cfg.CallInfo,
	rels flow.ReturnRelations,
	effects *assignCallPostconditionEffects,
) {
	for _, rel := range rels.LengthParams() {
		target, ok := assignmentTargetForReturn(info, callInfo, rel.ReturnIndex)
		if !ok {
			continue
		}
		targetRoot, targetPath, targetKey, ok := t.lengthPostconditionTarget(target)
		if !ok {
			continue
		}
		arg := callsite.RuntimeArgAt(callInfo, rel.ParamIndex)
		if arg == nil {
			continue
		}
		if paramIndex, ok := t.runtimeArgCallerParamIndex(arg); ok {
			effects.relations = append(effects.relations, flow.RelationEffect{
				Kind:       flow.RelationSeedTargetLengthParam,
				TargetRoot: targetRoot,
				TargetKey:  targetKey,
				ParamIndex: paramIndex,
			})
		}
		argKey, ok := t.containerExprKey(arg)
		if !ok {
			continue
		}
		lower := int64(0)
		if out.Num != nil {
			if numericLower, _, ok := out.Num.LenBoundsFor(argKey); ok && numericLower > lower {
				lower = numericLower
			}
		}
		if relationLower, ok := out.Rel.ContainerLowerBoundFor(argKey); ok && relationLower > lower {
			lower = relationLower
		}
		if lower <= 0 {
			continue
		}
		if op, ok := flow.NumericLenGeConstPathOp(targetPath, lower); ok {
			effects.numericOps = append(effects.numericOps, op)
		}
	}
}

func (t *Transfer) callBoundaryFacts(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	demand func(int, paramevidence.ParamContract),
) flow.BoundaryFacts {
	if out == nil || call == nil || t.callTyper == nil {
		return flow.BoundaryFactsDomain.Top()
	}
	provider, ok := t.callTyper.(productCallPostEffectProvider)
	if !ok || provider == nil {
		return flow.BoundaryFactsDomain.Top()
	}
	return provider.CallPostEffectsFromValues(call, t.productCallContext(out, call, demand)).BoundaryFacts
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

func (t *Transfer) lengthPostconditionTarget(target cfg.AssignTarget) (cfg.SymbolID, constraint.Path, constraint.PathKey, bool) {
	path, ok := t.staticPathOfAssignTarget(target)
	if !ok || path.Symbol == 0 {
		return 0, constraint.Path{}, "", false
	}
	key, ok := flow.SymbolPathKeyOf(path)
	if !ok {
		return 0, constraint.Path{}, "", false
	}
	return path.Symbol, path, key, true
}

func (t *Transfer) runtimeArgCallerParamIndex(arg ast.Expr) (int, bool) {
	path, ok := t.staticPathOfExpr(arg)
	if !ok || path.Symbol == 0 || len(path.Segments) != 0 {
		return 0, false
	}
	idx, ok := t.paramBySym[path.Symbol]
	return idx, ok
}
