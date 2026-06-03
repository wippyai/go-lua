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
	relations  []RelationEffect
	numericOps []NumericOp
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
		t.appendLengthParamPostconditions(out, info, callInfo, rels, &effects)
	})
	return effects
}

func (t *Transfer) applyAssignCallPostconditions(out *flow.PointState, effects assignCallPostconditionEffects) {
	if out == nil {
		return
	}
	for _, rel := range effects.relations {
		t.applyRelationEffect(out, rel)
	}
	if len(effects.numericOps) > 0 {
		t.applyNumericEffect(out, NumericEffect{Ops: effects.numericOps})
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
		effects.relations = append(effects.relations, RelationEffect{
			Kind:      RelationSeedSiblingNil,
			ErrSym:    errTarget.Symbol,
			ValueSyms: []cfg.SymbolID{valTarget.Symbol},
		})
	}
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
		targetRoot, targetKey, ok := t.lengthPostconditionTarget(target)
		if !ok {
			continue
		}
		arg := callsite.RuntimeArgAt(callInfo, rel.ParamIndex)
		if arg == nil {
			continue
		}
		if paramIndex, ok := t.runtimeArgCallerParamIndex(arg); ok {
			effects.relations = append(effects.relations, RelationEffect{
				Kind:       RelationSeedTargetLengthParam,
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
		effects.numericOps = append(effects.numericOps, NumericOp{
			Kind:  NumericLenGeConst,
			Key:   targetKey,
			Const: lower,
		})
	}
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

func (t *Transfer) lengthPostconditionTarget(target cfg.AssignTarget) (cfg.SymbolID, constraint.PathKey, bool) {
	path, ok := t.staticPathOfAssignTarget(target)
	if !ok || path.Symbol == 0 {
		return 0, "", false
	}
	return path.Symbol, flow.SymbolPathKey(path.Symbol, path.Segments), true
}

func (t *Transfer) runtimeArgCallerParamIndex(arg ast.Expr) (int, bool) {
	path, ok := t.staticPathOfExpr(arg)
	if !ok || path.Symbol == 0 || len(path.Segments) != 0 {
		return 0, false
	}
	idx, ok := t.paramBySym[path.Symbol]
	return idx, ok
}
