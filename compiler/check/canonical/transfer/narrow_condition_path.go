package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/cfg/extraction"
	domainpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// fieldResolver is the structural field/index resolver guard narrowing uses to
// read and rebuild structural paths. It is the pure value-domain resolver
// (types/query/core), not a parallel implementation.
var fieldResolver = querycore.Resolver()

// narrowByCondCheck applies the simple condition-check guard the CFG
// pre-extracted onto the branch. The conditionPathGuard owns the normalized
// symbol/path/check proof; this reducer applies that proof to PointState axes.
func (t *Transfer) narrowByCondCheck(out flow.PointState, info *cfg.BranchInfo, taken, atExit bool) flow.PointState {
	return t.narrowByCondCheckAtPoint(0, out, info, taken, atExit)
}

func (t *Transfer) narrowByCondCheckAtPoint(point cfg.Point, out flow.PointState, info *cfg.BranchInfo, taken, atExit bool) flow.PointState {
	check := effectiveCheck(info.CondCheck.Kind, taken)
	if check == cfg.CheckNone {
		return out
	}
	out = t.narrowGuardedIndexPresence(out, info, check)
	sym := t.condTestSymbol(info)
	segments := t.condTestSegments(info)
	if pathSym, pathSegments, ok := t.condTestPathInState(&out, info); ok {
		if sym == 0 {
			sym = pathSym
		}
		if len(pathSegments) > 0 {
			segments = pathSegments
		}
	}
	if sym == 0 {
		return out
	}
	if comparisonTruthyOnOperand(info.Condition, check) {
		return out
	}
	guard := conditionPathGuard{
		point:    point,
		sym:      sym,
		segments: segments,
		varPath:  info.CondVar,
		check:    check,
		typeName: info.CondCheck.TypeName,
	}
	currentAV, hasCurrent := t.symbolValue(&out, sym)
	seed := t.narrowSeed(sym, currentAV, atExit)
	baseAV, has := seed.value, seed.hasValue()

	cond := guard.condition(t)
	res := flow.ClonePointState(out)
	if cond.HasConstraints() {
		beforeAV, beforeOK := t.symbolValue(&res, sym)
		t.applyConditionEffect(&res, ConditionEffect{Fact: cond})
		if afterAV, afterOK := t.symbolValue(&res, sym); afterOK && !afterAV.IsZero() &&
			(!beforeOK || beforeAV.IsZero() || !product.Domain.Equal(beforeAV, afterAV)) {
			baseAV = afterAV
			has = true
		}
		if flow.PointStateDomain.Equal(res, flow.PointStateDomain.Bottom()) {
			return res
		}
	}
	res = t.narrowIndexPresenceLength(res, guard.sym, guard.segments, guard.check)
	guard.refineStaticMemberFact(t, &res, baseAV, has)
	if !has {
		if guard.narrowsBareSymbolPresence() {
			if t.unannotatedParam.Contains(guard.sym) {
				return res
			}
			t.setNarrowedSymbol(&res, guard.sym, product.PresentDynamic())
		}
		return res
	}
	narrowed, ok := guard.narrowValue(baseAV)
	if !ok {
		return res
	}
	narrowedBase := baseAV
	if seed.fromDeclared() && hasCurrent && !currentAV.IsZero() && !currentAV.Covers(narrowed) {
		currentNarrowed, currentOK := guard.narrowValue(currentAV)
		if currentOK && !valueIsBottom(currentNarrowed) &&
			guard.authorizesCurrentSeed(t, &res, seed.value, currentAV) {
			narrowed = currentNarrowed
			narrowedBase = currentAV
		}
	} else if !seed.fromDeclared() && hasCurrent && !currentAV.IsZero() && !currentAV.Covers(narrowed) {
		currentNarrowed, currentOK := guard.narrowValue(currentAV)
		if !currentOK || !flow.SemanticProductReduction(currentAV, currentNarrowed) {
			return res
		}
		narrowed = currentNarrowed
		narrowedBase = currentAV
	}
	if valueIsBottom(narrowed) && missingStaticMemberGuardStaysDynamic(narrowedBase, guard.segments, guard.check, guard.typeName) {
		return res
	}
	t.setNarrowedSymbol(&res, guard.sym, narrowed)
	return res
}

func (t *Transfer) condTestPathInState(out *flow.PointState, info *cfg.BranchInfo) (cfg.SymbolID, []constraint.Segment, bool) {
	expr := condCheckedExpr(info)
	if expr == nil {
		return 0, nil, false
	}
	return t.pathSymbolInState(out, expr, nil)
}

func condCheckedExpr(info *cfg.BranchInfo) ast.Expr {
	if info == nil {
		return nil
	}
	switch info.CondCheck.Kind {
	case cfg.CheckTruthy:
		return info.Condition
	case cfg.CheckFalsy:
		if not, ok := info.Condition.(*ast.UnaryNotOpExpr); ok {
			return not.Expr
		}
		return info.Condition
	case cfg.CheckNil, cfg.CheckNotNil:
		if rel, ok := info.Condition.(*ast.RelationalOpExpr); ok {
			if _, ok := rel.Lhs.(*ast.NilExpr); ok {
				return rel.Rhs
			}
			if _, ok := rel.Rhs.(*ast.NilExpr); ok {
				return rel.Lhs
			}
		}
	}
	return nil
}

func (t *Transfer) resolveTypeKey(key narrow.TypeKey) typ.Type {
	if key.Kind == narrow.TypeKeyBuiltin {
		builtin, ok := key.BuiltinKind()
		if !ok {
			return nil
		}
		return narrow.TypeForKind(builtin)
	}
	if t == nil || t.typeKey == nil {
		return nil
	}
	return t.typeKey(key)
}

func (t *Transfer) narrowGuardedIndexPresence(out flow.PointState, info *cfg.BranchInfo, check cfg.CondCheckKind) flow.PointState {
	access := guardedIndexPresenceAccess(info, check)
	if access == nil {
		return out
	}
	effect, ok := t.guardedIndexPresenceTransaction(access)
	if !ok {
		return out
	}
	res := out
	t.applyKeyProvenancePathTransaction(&res, effect)
	return res
}

func (t *Transfer) guardedIndexPresenceTransaction(access *ast.AttrGetExpr) (flow.KeyProvenancePathTransaction, bool) {
	if access == nil {
		return flow.KeyProvenancePathTransaction{}, false
	}
	if _, isStatic := staticMemberKey(access); isStatic {
		return flow.KeyProvenancePathTransaction{}, false
	}
	tablePath, ok := t.containerExprPath(access.Object)
	if !ok || tablePath.IsEmpty() {
		return flow.KeyProvenancePathTransaction{}, false
	}
	keyPath, ok := t.dynamicIndexKeyPath(access.Key)
	if !ok || keyPath.IsEmpty() {
		return flow.KeyProvenancePathTransaction{}, false
	}
	return flow.KeyProvenancePathTransaction{
		Kind:      flow.KeyProvenanceGuardedIndex,
		TablePath: tablePath,
		KeyPath:   keyPath,
	}, true
}

func guardedIndexPresenceAccess(info *cfg.BranchInfo, check cfg.CondCheckKind) *ast.AttrGetExpr {
	if info == nil || !checkProvesIndexPresence(check) {
		return nil
	}
	switch info.CondCheck.Kind {
	case cfg.CheckTruthy:
		return attrAccess(info.Condition)
	case cfg.CheckFalsy:
		if not, ok := info.Condition.(*ast.UnaryNotOpExpr); ok {
			return attrAccess(not.Expr)
		}
	case cfg.CheckNil, cfg.CheckNotNil:
		if rel, ok := info.Condition.(*ast.RelationalOpExpr); ok {
			return nilComparisonAttrAccess(rel)
		}
	}
	return nil
}

func checkProvesIndexPresence(check cfg.CondCheckKind) bool {
	return check == cfg.CheckTruthy || check == cfg.CheckNotNil
}

func attrAccess(expr ast.Expr) *ast.AttrGetExpr {
	attr, _ := expr.(*ast.AttrGetExpr)
	return attr
}

func nilComparisonAttrAccess(rel *ast.RelationalOpExpr) *ast.AttrGetExpr {
	if rel == nil {
		return nil
	}
	if _, ok := rel.Rhs.(*ast.NilExpr); ok {
		return attrAccess(rel.Lhs)
	}
	if _, ok := rel.Lhs.(*ast.NilExpr); ok {
		return attrAccess(rel.Rhs)
	}
	return nil
}

func staticMemberGuardImpliesPresence(check cfg.CondCheckKind, typeName string) bool {
	switch check {
	case cfg.CheckTruthy, cfg.CheckNotNil:
		return true
	case cfg.CheckTypeEqual:
		return kind.FromString(typeName) != kind.Nil && kind.FromString(typeName) != kind.Unknown
	case cfg.CheckTypeNot:
		return kind.FromString(typeName) == kind.Nil
	default:
		return false
	}
}

func missingStaticMemberGuardStaysDynamic(base product.AbstractValue, segments []constraint.Segment, check cfg.CondCheckKind, typeName string) bool {
	if len(segments) == 0 || !staticMemberGuardImpliesPresence(check, typeName) || base.IsZero() {
		return false
	}
	baseType := base.ProjectValue()
	return !fieldPathResolves(baseType, segments) && querycore.MissingFieldReadsNil(baseType)
}

func fieldPathResolves(t typ.Type, segments []constraint.Segment) bool {
	if t == nil || len(segments) == 0 {
		return false
	}
	current := t
	for _, seg := range segments {
		if seg.Kind != constraint.SegmentField && seg.Kind != constraint.SegmentIndexString {
			return false
		}
		next, ok := fieldResolver.Field(current, seg.Name)
		if !ok || next == nil {
			return false
		}
		current = next
	}
	return true
}

func (t *Transfer) condTestSegments(info *cfg.BranchInfo) []constraint.Segment {
	if seg := t.condTestSegmentsFromAST(info); seg != nil {
		return seg
	}
	root := extraction.ExtractRootName(info.CondVar)
	if root == "" || root == info.CondVar {
		return nil
	}
	return pathkey.ParseSuffix(info.CondVar[len(root):])
}

func (t *Transfer) condTestSegmentsFromAST(info *cfg.BranchInfo) []constraint.Segment {
	switch info.CondCheck.Kind {
	case cfg.CheckTruthy:
		return staticAccessSegments(info.Condition)
	case cfg.CheckFalsy:
		if not, ok := info.Condition.(*ast.UnaryNotOpExpr); ok {
			return staticAccessSegments(not.Expr)
		}
	case cfg.CheckNil, cfg.CheckNotNil:
		if rel, ok := info.Condition.(*ast.RelationalOpExpr); ok {
			if seg := staticAccessSegments(rel.Lhs); seg != nil {
				return seg
			}
			return staticAccessSegments(rel.Rhs)
		}
	case cfg.CheckTypeEqual, cfg.CheckTypeNot:
		return t.typeofArgSegments(info.Condition)
	}
	return nil
}

func staticAccessSegments(expr ast.Expr) []constraint.Segment {
	segs, ok := staticSegmentsOfExpr(expr)
	if !ok || len(segs) == 0 {
		return nil
	}
	return segs
}

func (t *Transfer) typeofArgSegments(expr ast.Expr) []constraint.Segment {
	rel, ok := expr.(*ast.RelationalOpExpr)
	if !ok {
		return nil
	}
	for _, side := range []ast.Expr{rel.Lhs, rel.Rhs} {
		call, ok := side.(*ast.FuncCallExpr)
		if !ok || call.Method != "" || call.Receiver != nil || len(call.Args) != 1 {
			continue
		}
		fn, ok := call.Func.(*ast.IdentExpr)
		if !ok || fn.Value != "type" {
			continue
		}
		return staticAccessSegments(call.Args[0])
	}
	return nil
}

func narrowAtPath(av product.AbstractValue, segments []constraint.Segment, check cfg.CondCheckKind, typeName string) (product.AbstractValue, bool) {
	if len(segments) == 0 {
		return narrowValue(av, check, typeName)
	}
	base := av.ProjectValue()
	if base == nil {
		return product.AbstractValue{}, false
	}
	refined := narrowFieldPath(base, segments, check, typeName)
	if refined == nil || refined == base {
		return product.AbstractValue{}, false
	}
	if refined.Kind().IsNever() {
		return product.Bottom(), true
	}
	return product.FromType(refined), true
}

func narrowFieldPath(t typ.Type, segments []constraint.Segment, check cfg.CondCheckKind, typeName string) typ.Type {
	if t == nil || len(segments) == 0 {
		return t
	}
	seg := segments[0]
	if seg.Kind != constraint.SegmentField && seg.Kind != constraint.SegmentIndexString {
		return t
	}
	if len(segments) == 1 {
		refine, absentKeeps, ok := fieldRefiner(check, typeName)
		if !ok {
			return t
		}
		return mapUnionField(t, seg.Name, refine, absentKeeps)
	}
	return mapUnionField(t, seg.Name, func(ft typ.Type) typ.Type {
		return narrowFieldPath(ft, segments[1:], check, typeName)
	}, false)
}

func fieldRefiner(check cfg.CondCheckKind, typeName string) (refine func(typ.Type) typ.Type, absentKeeps bool, ok bool) {
	switch check {
	case cfg.CheckTruthy:
		return narrow.ToTruthy, false, true
	case cfg.CheckFalsy:
		return narrow.ToFalsy, true, true
	case cfg.CheckNotNil:
		return narrow.RemoveNil, false, true
	case cfg.CheckNil:
		return func(typ.Type) typ.Type { return typ.Nil }, true, true
	case cfg.CheckTypeEqual:
		key, known := narrow.KnownBuiltinTypeKey(typeName)
		if !known {
			return nil, false, false
		}
		return func(ft typ.Type) typ.Type { return narrow.ByTypeKey(ft, key, nil) }, false, true
	case cfg.CheckTypeNot:
		key, known := narrow.KnownBuiltinTypeKey(typeName)
		if !known {
			return nil, false, false
		}
		return func(ft typ.Type) typ.Type { return narrow.ExcludeByTypeKey(ft, key, nil) }, true, true
	default:
		return nil, false, false
	}
}

func mapUnionField(t typ.Type, field string, refine func(typ.Type) typ.Type, absentKeeps bool) typ.Type {
	if t == nil {
		return t
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(t)
		if expanded == nil || expanded == t {
			return t
		}
		return mapUnionField(expanded, field, refine, absentKeeps)
	case *typ.Union:
		kept := make([]typ.Type, 0, len(v.Members))
		for _, m := range v.Members {
			refined := mapUnionField(m, field, refine, absentKeeps)
			if refined == nil || refined.Kind().IsNever() {
				continue
			}
			kept = append(kept, refined)
		}
		if len(kept) == 0 {
			return typ.Never
		}
		return typ.NewUnion(kept...)
	case *typ.Optional:
		return mapUnionField(v.Inner, field, refine, absentKeeps)
	case *typ.Intersection:
		found := false
		members := make([]typ.Type, len(v.Members))
		for i, m := range v.Members {
			if _, ok := fieldResolver.Field(m, field); !ok {
				members[i] = m
				continue
			}
			found = true
			refined := mapUnionField(m, field, refine, absentKeeps)
			if refined == nil || refined.Kind().IsNever() {
				return typ.Never
			}
			members[i] = refined
		}
		if !found {
			if absentKeeps {
				return t
			}
			return typ.Never
		}
		return typ.NewIntersection(members...)
	case *typ.Record:
		ft, ok := fieldResolver.Field(v, field)
		if !ok || ft == nil {
			if absentKeeps {
				return t
			}
			return typ.Never
		}
		refined := refine(ft)
		if refined == nil || refined.Kind().IsNever() {
			return typ.Never
		}
		return typ.ExtendRecordWithField(v, field, refined)
	case *typ.Map:
		refined := refine(v.Value)
		if refined == nil {
			return t
		}
		if refined.Kind().IsNever() {
			if absentKeeps {
				return t
			}
			return typ.Never
		}
		return typ.NewRecord().SetOpen(true).Field(field, refined).MapComponent(v.Key, v.Value).Build()
	default:
		if typ.IsAny(t) {
			refined := refine(typ.Any)
			if refined == nil {
				return t
			}
			if refined.Kind().IsNever() {
				if absentKeeps {
					return t
				}
				return typ.Never
			}
			if !gradualAnyFieldProofAdmissible(refined) {
				return t
			}
			return typ.NewRecord().SetOpen(true).Field(field, refined).MapComponent(typ.Any, typ.Any).Build()
		}
		ft, ok := fieldResolver.Field(t, field)
		if !ok || ft == nil {
			return t
		}
		refined := refine(ft)
		if refined == nil || refined.Kind().IsNever() {
			return typ.Never
		}
		return t
	}
}

func gradualAnyFieldProofAdmissible(refined typ.Type) bool {
	if refined == nil {
		return false
	}
	if refined.Kind().IsPrimitive() {
		return true
	}
	rec, ok := unwrap.Alias(refined).(*typ.Record)
	return ok && rec != nil
}

func (t *Transfer) condTestSymbol(info *cfg.BranchInfo) cfg.SymbolID {
	if info.CondSymbol != 0 {
		return info.CondSymbol
	}
	if info.CondCheck.Kind == cfg.CheckTypeEqual || info.CondCheck.Kind == cfg.CheckTypeNot {
		return t.typeofArgSymbol(info.Condition)
	}
	return 0
}

func (t *Transfer) typeofArgSymbol(expr ast.Expr) cfg.SymbolID {
	rel, ok := expr.(*ast.RelationalOpExpr)
	if !ok {
		return 0
	}
	for _, side := range []ast.Expr{rel.Lhs, rel.Rhs} {
		call, ok := side.(*ast.FuncCallExpr)
		if !ok || call.Method != "" || call.Receiver != nil || len(call.Args) != 1 {
			continue
		}
		fn, ok := call.Func.(*ast.IdentExpr)
		if !ok || fn.Value != "type" {
			continue
		}
		if sym, _, ok := t.pathSymbol(call.Args[0]); ok {
			return sym
		}
	}
	return 0
}

func effectiveCheck(k cfg.CondCheckKind, taken bool) cfg.CondCheckKind {
	if taken {
		switch k {
		case cfg.CheckTruthy, cfg.CheckFalsy, cfg.CheckNil, cfg.CheckNotNil, cfg.CheckTypeEqual, cfg.CheckTypeNot:
			return k
		default:
			return cfg.CheckNone
		}
	}
	switch k {
	case cfg.CheckTruthy:
		return cfg.CheckFalsy
	case cfg.CheckFalsy:
		return cfg.CheckTruthy
	case cfg.CheckNil:
		return cfg.CheckNotNil
	case cfg.CheckNotNil:
		return cfg.CheckNil
	case cfg.CheckTypeEqual:
		return cfg.CheckTypeNot
	case cfg.CheckTypeNot:
		return cfg.CheckTypeEqual
	default:
		return cfg.CheckNone
	}
}

func narrowValue(av product.AbstractValue, check cfg.CondCheckKind, typeName string) (product.AbstractValue, bool) {
	switch check {
	case cfg.CheckTruthy:
		return product.NarrowTruthy(av), true
	case cfg.CheckFalsy:
		return product.NarrowFalsy(av), true
	case cfg.CheckNotNil:
		return product.NarrowPresent(av), true
	case cfg.CheckNil:
		return product.FromType(typ.Nil), true
	case cfg.CheckTypeEqual:
		k := kind.FromString(typeName)
		if k == kind.Unknown {
			return product.AbstractValue{}, false
		}
		return product.FilterByKind(av, k), true
	case cfg.CheckTypeNot:
		k := kind.FromString(typeName)
		if k == kind.Unknown {
			return product.AbstractValue{}, false
		}
		return product.ExcludeByKind(av, k), true
	default:
		return product.AbstractValue{}, false
	}
}

func (t *Transfer) conditionRefinedCaptureValue(out *flow.PointState, sym cfg.SymbolID, base product.AbstractValue, hasBase bool) (product.AbstractValue, bool) {
	if out == nil || sym == 0 || !out.Cond.HasConstraints() {
		return product.AbstractValue{}, false
	}
	next, ok := flow.ProductConditionReductionValue(flow.ProductConditionReduction{
		Symbol:   sym,
		Base:     base,
		HasBase:  hasBase,
		Fact:     out.Cond,
		Facts:    flow.PointFactsOfBorrowed(out),
		Resolver: fieldResolver,
	})
	if !ok || next.IsZero() {
		return product.AbstractValue{}, false
	}
	return next, true
}

func (t *Transfer) versionedPath(point cfg.Point, path constraint.Path) constraint.Path {
	return domainpath.WithVersion(path, t.in.Graph, point)
}

func (t *Transfer) versionedStaticPathOfExpr(point cfg.Point, expr ast.Expr) (constraint.Path, bool) {
	path, ok := t.staticPathOfExpr(expr)
	if !ok {
		return constraint.Path{}, false
	}
	return t.versionedPath(point, path), true
}

func typeKeyFor(typeName string) narrow.TypeKey {
	key, ok := narrow.KnownBuiltinTypeKey(typeName)
	if !ok {
		return narrow.TypeKey{}
	}
	return key
}
