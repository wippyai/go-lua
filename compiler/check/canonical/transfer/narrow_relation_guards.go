package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func (t *Transfer) narrowByBranchConditionEffect(point cfg.Point, out flow.PointState, info *cfg.BranchInfo, taken bool) flow.PointState {
	effect, ok := t.branchConditions.effect(point, out, info, taken)
	if !ok {
		return out
	}
	res := out
	t.applyBorrowedEdgeConditionEffect(&res, effect)
	return res
}

func (t *Transfer) conditionExtractorInputs() *flow.Inputs {
	if t.in.Graph == nil {
		return nil
	}
	return &flow.Inputs{
		Graph:               t.in.Graph,
		DeclaredTypes:       t.in.Scope.DeclaredTypes,
		ConstValues:         t.in.ConstValues,
		VariantFieldOrigins: t.in.VariantFieldOrigins,
	}
}

func (t *Transfer) conditionSymbolResolver(out *flow.PointState) func(cfg.Point, cfg.SymbolID) (typ.Type, bool) {
	return func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if sym == 0 {
			return nil, false
		}
		if out != nil {
			av, has := t.symbolValue(out, sym)
			if has && !av.IsZero() {
				if pt := av.ProjectValue(); !typ.IsAbsentOrUnknown(pt) {
					return pt, true
				}
			}
		}
		if t, ok := t.declaredTypes[sym]; ok && !typ.IsAbsentOrUnknown(t) {
			return t, true
		}
		return nil, false
	}
}

// narrowBySiblingNil strips nil from the value siblings of a multi-return error
// assignment when the branch proves the error symbol nil.
func (t *Transfer) narrowBySiblingNil(out flow.PointState, info *cfg.BranchInfo, taken bool) flow.PointState {
	sym := info.CondSymbol
	if sym == 0 {
		sym = t.condTestSymbol(info)
	}
	if sym == 0 {
		return out
	}
	bind, ok := out.Rel.SiblingNil(sym)
	if !ok {
		return out
	}
	if len(t.condTestSegments(info)) > 0 {
		return out
	}
	errIsNil := effectiveCheck(info.CondCheck.Kind, taken)
	switch errIsNil {
	case cfg.CheckNil, cfg.CheckFalsy:
	default:
		return out
	}
	res := flow.ClonePointStateForEdgeFactEffect(out)
	applied := false
	for _, vs := range bind.ValueSyms {
		if vs == 0 {
			continue
		}
		av, has := t.symbolValue(&res, vs)
		if !has || av.IsZero() {
			continue
		}
		t.setNarrowedSymbol(&res, vs, product.NarrowPresent(av))
		applied = true
	}
	if !applied {
		return out
	}
	return res
}

func (t *Transfer) narrowByGuardedType(out flow.PointState, info *cfg.BranchInfo, taken bool) flow.PointState {
	sym := info.CondSymbol
	if sym == 0 {
		sym = t.condTestSymbol(info)
	}
	if sym == 0 || len(t.condTestSegments(info)) > 0 {
		return out
	}
	check := effectiveCheck(info.CondCheck.Kind, taken)
	var guardTruthy bool
	switch check {
	case cfg.CheckTruthy, cfg.CheckNotNil:
		guardTruthy = true
	case cfg.CheckFalsy, cfg.CheckNil:
		guardTruthy = false
	default:
		return out
	}
	rels := out.Rel.GuardedTypesForGuard(sym, guardTruthy)
	if len(rels) == 0 {
		return out
	}
	res := flow.ClonePointStateForEdgeFactEffect(out)
	applied := false
	for _, rel := range rels {
		if rel.TargetSym == 0 || rel.TargetType == nil {
			continue
		}
		av, has := t.symbolValue(&res, rel.TargetSym)
		if !has || av.IsZero() {
			continue
		}
		refined := product.FromRefinedType(av, rel.TargetType)
		if refined.IsZero() {
			refined = product.FromType(rel.TargetType)
		}
		t.setNarrowedSymbol(&res, rel.TargetSym, refined)
		applied = true
	}
	if !applied {
		return out
	}
	return res
}

// narrowByTypeCheck applies the value-narrowing a `local val, err = T:is(x)` guard
// proves on the successful error-nil edge.
func (t *Transfer) narrowByTypeCheck(out flow.PointState, info *cfg.BranchInfo, taken bool) (flow.PointState, bool) {
	if t.typeCheckByErr == nil || info.CondSymbol == 0 {
		return out, false
	}
	bind, ok := t.typeCheckByErr[info.CondSymbol]
	if !ok {
		return out, false
	}
	errIsNil := effectiveCheck(info.CondCheck.Kind, taken)
	switch errIsNil {
	case cfg.CheckNil, cfg.CheckFalsy:
	default:
		return out, false
	}
	val := product.FromType(bind.Type)
	if val.IsZero() {
		return out, false
	}
	res := flow.ClonePointStateForEdgeFactEffect(out)
	for _, sym := range bind.NarrowSyms {
		if sym == 0 {
			continue
		}
		t.setNarrowedSymbol(&res, sym, val)
	}
	return res, true
}

// narrowByPredicate applies the value-narrowing a local type-predicate guard
// proves. Predicate narrowing is one-sided: only the edge on which the predicate
// result holds true narrows the argument.
func (t *Transfer) narrowByPredicate(out flow.PointState, info *cfg.BranchInfo, taken bool) flow.PointState {
	if len(t.predicateByFunc) == 0 && len(t.predicateByCondSym) == 0 {
		return out
	}
	argSym, kind, trueEdge, ok := t.predicateGuardForBranch(info, taken)
	if !ok || argSym == 0 || kind == "" || !trueEdge {
		return out
	}
	if _, known := narrow.KnownBuiltinTypeKey(kind); !known {
		return out
	}
	baseAV, has := t.narrowBaseFor(out, argSym, false)
	if !has {
		return out
	}
	narrowed, ok := narrowAtPath(baseAV, nil, cfg.CheckTypeEqual, kind)
	if !ok {
		return out
	}
	res := flow.ClonePointStateForEdgeFactEffect(out)
	t.setNarrowedSymbol(&res, argSym, narrowed)
	return res
}

func (t *Transfer) predicateGuardForBranch(info *cfg.BranchInfo, taken bool) (argSym cfg.SymbolID, kind string, trueEdge, ok bool) {
	if info.CondSymbol != 0 {
		if g, found := t.predicateByCondSym[info.CondSymbol]; found {
			if len(t.condTestSegments(info)) > 0 {
				return 0, "", false, false
			}
			check := effectiveCheck(info.CondCheck.Kind, taken)
			return g.NarrowSym, g.Kind, check == cfg.CheckTruthy, true
		}
	}
	cond := info.Condition
	trueEdge = taken
	if not, isNot := cond.(*ast.UnaryNotOpExpr); isNot {
		cond = not.Expr
		trueEdge = !taken
	}
	call, isCall := cond.(*ast.FuncCallExpr)
	if !isCall || call == nil {
		return 0, "", false, false
	}
	sym, kindName, found := t.predicateCallNarrow(call)
	if !found {
		return 0, "", false, false
	}
	return sym, kindName, trueEdge, true
}

func (t *Transfer) predicateCallNarrow(call *ast.FuncCallExpr) (argSym cfg.SymbolID, kind string, found bool) {
	if call == nil || len(t.predicateByFunc) == 0 {
		return 0, "", false
	}
	fnIdent, ok := call.Func.(*ast.IdentExpr)
	if !ok || fnIdent == nil {
		return 0, "", false
	}
	fnSym := t.symbolOf(fnIdent)
	if fnSym == 0 {
		return 0, "", false
	}
	fact, ok := t.predicateByFunc[fnSym]
	if !ok || fact.Kind == "" {
		return 0, "", false
	}
	if fact.ParamIndex < 0 || fact.ParamIndex >= len(call.Args) {
		return 0, "", false
	}
	argIdent, ok := call.Args[fact.ParamIndex].(*ast.IdentExpr)
	if !ok || argIdent == nil {
		return 0, "", false
	}
	sym := t.symbolOf(argIdent)
	if sym == 0 {
		return 0, "", false
	}
	return sym, fact.Kind, true
}
