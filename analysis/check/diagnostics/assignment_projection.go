package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics/internal/readmodel"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/transform"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse/numparse"
)

func assignmentValueType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr, source sourceprovenance.ASTSource) (typ.Type, bool) {
	if got, ok := valueexpr.LiteralType(expr); ok {
		return got, true
	}
	if got, ok := projectedOptionalIndexType(result, resolver, point, expr); ok {
		return got, true
	}
	if got, ok := untrustedTopLikeExpressionTypeAt(result, resolver, point, expr); ok {
		return got, true
	}
	if got, ok := sourceExpressionTypeWithPresence(result, point, source); ok {
		return got, true
	}
	if got, ok := readmodel.New(result).SourceType(point, source); ok {
		return got, true
	}
	if got, ok := explicitTopLikeCallSourceType(result, resolver, expr); ok {
		return got, true
	}
	if got, ok := explicitTopLikeCallFactSourceType(result, resolver, source); ok {
		return got, true
	}
	return boundaryExprType(result, resolver, expr)
}

func sourceExpressionTypeWithPresence(result *body.Result, point cfg.Point, source sourceprovenance.ASTSource) (typ.Type, bool) {
	if source.Kind != sourceprovenance.SourceExpression || !presenceAwareReadExpression(source.Expr) {
		return nil, false
	}
	reader := readmodel.New(result)
	value, ok := reader.SourceValue(point, source)
	if !ok {
		return nil, false
	}
	return reader.ValueTypeWithPresence(value)
}

func presenceAwareReadExpression(expr ast.Expr) bool {
	_, ok := expr.(*ast.AttrGetExpr)
	return ok
}

func assignmentTargetType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, fact semantics.OrdinaryAssignmentFact) (typ.Type, bool) {
	if fact.HasPath && fact.Path.Symbol != 0 && len(fact.Path.Segments) > 0 {
		return newExpressionTyper(result, resolver).typeOf(fact.Target)
	}
	if !fact.HasContainerPath || fact.ContainerPath.Symbol == 0 {
		return nil, false
	}
	attr, ok := fact.Target.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return nil, false
	}
	return dynamicIndexAssignmentTargetType(result, resolver, point, attr)
}

func inferredFunctionValueType(result *body.Result, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	fn, ok := result.FunctionValueTypeAtBoundary(point, expr)
	if !ok {
		return nil, false
	}
	return fn, true
}

func dynamicIndexAssignmentTargetType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, attr *ast.AttrGetExpr) (typ.Type, bool) {
	typer := newExpressionTyper(result, resolver)
	if t, ok := typer.typeOf(attr); ok {
		return t, true
	}
	if attr == nil || attr.Key == nil {
		return nil, false
	}
	if _, ok := typer.typeOf(attr.Key); ok {
		return nil, false
	}
	container, ok := containerFlowType(result, resolver, point, attr.Object)
	if !ok {
		return nil, false
	}
	return dynamicIndexWriteValueType(container, 0)
}

// containerFlowType resolves the flow-sensitive type of a dynamic-index write's
// container expression. The static typer cannot type an unannotated local whose
// shape is only known through flow, so the boundary value is consulted before
// falling back to the static type.
func containerFlowType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, object ast.Expr) (typ.Type, bool) {
	if value, ok := result.ExpressionValueAtBoundary(point, object); ok {
		if t, ok := readmodel.New(result).ValueType(value); ok {
			return t, true
		}
	}
	return newExpressionTyper(result, resolver).typeOf(object)
}

func dynamicIndexWriteValueType(t typ.Type, depth int) (typ.Type, bool) {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch tt := transparentExpectedType(t).(type) {
	case *typ.Optional:
		return dynamicIndexWriteValueType(tt.Inner, depth+1)
	case *typ.Union:
		var members []typ.Type
		for _, member := range tt.Members {
			value, ok := dynamicIndexWriteValueType(member, depth+1)
			if !ok {
				continue
			}
			members = append(members, value)
		}
		if len(members) == 0 {
			return nil, false
		}
		return normalize.UnionForEvidence(members...), true
	case *typ.Intersection:
		var members []typ.Type
		for _, member := range tt.Members {
			value, ok := dynamicIndexWriteValueType(member, depth+1)
			if !ok {
				continue
			}
			members = append(members, value)
		}
		if len(members) == 0 {
			return nil, false
		}
		return normalize.IntersectionForMeet(members...), true
	case *typ.Record:
		if tt.HasMapComponent() && tt.MapValue != nil {
			return tt.MapValue, true
		}
		return closedRecordDynamicWriteValueType(tt)
	case *typ.Map:
		if tt.Value != nil {
			return tt.Value, true
		}
	case *typ.ReadonlyMap:
		if tt.Value != nil {
			return tt.Value, true
		}
	case *typ.Array:
		if tt.Element != nil {
			return tt.Element, true
		}
	}
	return nil, false
}

// closedRecordDynamicWriteValueType resolves the admissible value type for a
// dynamic-key write into a closed record without a map component. A dynamic
// string key could match any declared field, so a sound write must produce a
// value assignable to every field the key could land on: the admissible type is
// the meet of the field domains. Records with no concrete fields carry no
// dynamic-write contract here.
func closedRecordDynamicWriteValueType(record *typ.Record) (typ.Type, bool) {
	if record == nil || record.Open || len(record.Fields) == 0 {
		return nil, false
	}
	members := make([]typ.Type, 0, len(record.Fields))
	for _, field := range record.Fields {
		if field.Type == nil {
			return nil, false
		}
		members = append(members, field.Type)
	}
	if len(members) == 1 {
		return members[0], true
	}
	return normalize.IntersectionForMeet(members...), true
}

func clearMismatch(result *body.Result, got, want typ.Type) bool {
	if got == nil || want == nil || typ.IsAny(got) || typ.IsUnknown(got) || typ.IsAny(want) || typ.IsUnknown(want) {
		return false
	}
	if subtype.IsSubtype(got, want) {
		return false
	}
	if sameNominalGenericInstantiation(got, want) {
		return true
	}
	got = transparentComparableType(result, got)
	want = transparentComparableType(result, want)
	if subtype.IsSubtype(got, want) {
		return false
	}
	if explicitNilFieldFreshAssignable(got, want) {
		return false
	}
	if projectionHasNil(want) {
		nonNilWant := projectionWithoutNil(want)
		if nonNilWant != nil && !typ.IsNever(nonNilWant) &&
			subtype.IsSubtype(got, transparentComparableType(result, nonNilWant)) {
			return false
		}
	}
	return true
}

func explicitNilFieldFreshAssignable(got, want typ.Type) bool {
	return subtype.IsFreshAssignable(got, want) && hasExplicitNilToNilableMember(got, want, 0)
}

func hasExplicitNilToNilableMember(got, want typ.Type, depth int) bool {
	if got == nil || want == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	got = transparentExpectedType(got)
	want = transparentExpectedType(want)
	switch wt := want.(type) {
	case *typ.Optional:
		return hasExplicitNilToNilableMember(got, wt.Inner, depth+1)
	case *typ.Union:
		for _, member := range wt.Members {
			if subtype.IsFreshAssignable(got, member) && hasExplicitNilToNilableMember(got, member, depth+1) {
				return true
			}
		}
		return false
	}
	gotRecord, ok := got.(*typ.Record)
	if !ok || gotRecord == nil {
		return false
	}
	wantRecord, ok := want.(*typ.Record)
	if !ok || wantRecord == nil {
		return false
	}
	for _, field := range gotRecord.Fields {
		if field.Type == nil || field.Type.Kind() != kind.Nil {
			continue
		}
		wantField := wantRecord.GetField(field.Name)
		if wantField != nil && (wantField.Optional || projectionHasNil(wantField.Type)) {
			return true
		}
	}
	for _, member := range gotRecord.StaticMembers {
		if member.Type == nil || member.Type.Kind() != kind.Nil {
			continue
		}
		wantMember := wantRecord.GetStaticMember(member.Kind, member.Name, member.Index)
		if wantMember != nil && (wantMember.Optional || projectionHasNil(wantMember.Type)) {
			return true
		}
	}
	return false
}

func sameNominalGenericInstantiation(left, right typ.Type) bool {
	leftInst, leftOK := transparentInstantiation(left)
	rightInst, rightOK := transparentInstantiation(right)
	return leftOK && rightOK &&
		leftInst.Generic != nil &&
		rightInst.Generic != nil &&
		typ.TypeEquals(leftInst.Generic, rightInst.Generic) &&
		len(leftInst.TypeArgs) == len(rightInst.TypeArgs)
}

func transparentInstantiation(t typ.Type) (*typ.Instantiated, bool) {
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
		switch tt := t.(type) {
		case *typ.Annotated:
			if tt.Inner == nil || tt.Inner == t {
				return nil, false
			}
			t = tt.Inner
		case *typ.Alias:
			next := tt.UnaliasedTarget()
			if next == nil || next == t {
				return nil, false
			}
			t = next
		case *typ.Recursive:
			if tt.Body == nil || tt.Body == t {
				return nil, false
			}
			t = tt.Body
		case *typ.Instantiated:
			return tt, true
		default:
			return nil, false
		}
	}
	return nil, false
}

func transparentComparableType(result *body.Result, t typ.Type) typ.Type {
	t = transparentExpectedType(t)
	if result == nil {
		return t
	}
	moduleTypes := result.ModuleTypes()
	if len(moduleTypes.Manifests) == 0 {
		return t
	}
	resolved := transform.Rewrite(t, func(node typ.Type) (typ.Type, bool) {
		ref, ok := node.(*typ.Ref)
		if !ok || ref.Module == "" {
			return nil, false
		}
		if resolved, ok := moduleTypes.Lookup(ref.Module, ref.Name); ok {
			return resolved, true
		}
		if modulePath, ok := result.RequireAliasModulePath(ref.Module); ok {
			if resolved, ok := moduleTypes.Lookup(modulePath, ref.Name); ok {
				return resolved, true
			}
		}
		return nil, false
	})
	return transparentExpectedType(resolved)
}

func localScalarOperatorSourceType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	if !isScalarOperatorExpression(expr) {
		return nil, false
	}
	return newExpressionTyper(result, resolver).typeOf(expr)
}

func isScalarOperatorExpression(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.LogicalOpExpr:
		return true
	case *ast.RelationalOpExpr:
		return true
	case *ast.StringConcatOpExpr:
		return true
	case *ast.ArithmeticOpExpr:
		return true
	case *ast.UnaryMinusOpExpr:
		return true
	case *ast.UnaryNotOpExpr:
		return true
	case *ast.UnaryLenOpExpr:
		return true
	case *ast.UnaryBNotOpExpr:
		return true
	case *ast.CastExpr:
		return isScalarOperatorExpression(e.Expr)
	case *ast.NonNilAssertExpr:
		return isScalarOperatorExpression(e.Expr)
	default:
		return false
	}
}

func projectedOptionalIndexType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if !shouldProjectOptionalIndex(result, expr) {
		return nil, false
	}
	got, ok := newExpressionTyper(result, resolver).typeOf(expr)
	if !ok || !projectionHasNil(got) {
		return nil, false
	}
	if indexReadProvenInRange(result, point, expr) {
		withoutNil := projectionWithoutNil(got)
		if withoutNil != nil && !typ.IsNever(withoutNil) {
			return withoutNil, true
		}
	}
	return got, true
}

// indexReadProvenInRange reports whether an array element read attr is provably
// in range at point: the index is #array over the same container and a proven
// length floor establishes len(array) >= 1, so the element is non-optional.
func indexReadProvenInRange(result *body.Result, point cfg.Point, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return false
	}
	containerPath, ok := result.ExpressionPath(attr.Object)
	if !ok || containerPath.IsEmpty() {
		return false
	}
	if lenOp, ok := attr.Key.(*ast.UnaryLenOpExpr); ok {
		lenPath, ok := result.ExpressionPath(lenOp.Expr)
		if !ok || lenPath.Key() != containerPath.Key() {
			return false
		}
		floor, ok := result.LengthFloorAtBoundary(point, containerPath)
		return ok && floor >= 1
	}
	return symbolicIndexReadProvenInRange(result, point, attr.Key, containerPath)
}

func symbolicIndexReadProvenInRange(result *body.Result, point cfg.Point, index ast.Expr, containerPath pathdom.Path) bool {
	indexPath, ok := result.ExpressionPath(index)
	if !ok || indexPath.IsEmpty() {
		return false
	}
	if !result.IndexInRangeAtBoundary(point, indexPath, containerPath) {
		return false
	}
	return indexValueKnownPositive(result, point, index, indexPath)
}

func indexValueKnownPositive(result *body.Result, point cfg.Point, index ast.Expr, indexPath pathdom.Path) bool {
	num, ok := index.(*ast.NumberExpr)
	if ok {
		value, ok := numparse.ParseIntegerLiteral(num.Value)
		return ok && value >= 1
	}
	floor, ok := result.NumericFloorAtBoundary(point, indexPath)
	return ok && floor >= 1
}

func shouldProjectOptionalIndex(result *body.Result, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return false
	}
	if _, literal := attr.Key.(*ast.NumberExpr); !literal {
		return true
	}
	container, ok := result.ExpressionPath(attr.Object)
	return ok && len(container.Segments) > 0
}

// optionalMemberReadType types a dot-field read whose object is a call result
// (store:lookup_record(id).field) when no symbol-path or index projection owns it.
// The call result carries its own boundary presence, so reading a field off an
// optional result yields an optional projection that an annotated assignment must
// not silently accept. It returns the type only when it is provably optional, so
// a non-optional projection never produces a spurious source type here.
func optionalMemberReadType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, env literalEnv, expr ast.Expr) (typ.Type, bool) {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyDot {
		return nil, false
	}
	if _, ok := attr.Object.(*ast.FuncCallExpr); !ok {
		return nil, false
	}
	got, ok := newFlowExpressionTyper(result, resolver, point, env).typeOf(expr)
	if !ok || got == nil {
		return nil, false
	}
	if typ.IsAny(got) || typ.IsUnknown(got) || typ.IsNever(got) {
		return nil, false
	}
	if !projectionHasNil(got) {
		return nil, false
	}
	return got, true
}

func projectedFlowSourceType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, env literalEnv, expr ast.Expr) (typ.Type, bool) {
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		if e.KeySyntax == ast.AttrKeyIndex && !shouldProjectOptionalIndex(result, e) {
			return nil, false
		}
		got, ok := newFlowExpressionTyper(result, resolver, point, env).typeOf(expr)
		if !ok {
			return nil, false
		}
		raw, rawOK := newExpressionTyper(result, resolver).typeOf(expr)
		if !rawOK || !typ.SameNodeOrAcyclicEqual(got, raw) {
			return got, true
		}
		return nil, false
	case *ast.IdentExpr:
		got, ok := newFlowExpressionTyper(result, resolver, point, env).typeOf(expr)
		if !ok {
			return nil, false
		}
		raw, rawOK := newExpressionTyper(result, resolver).typeOf(expr)
		if !rawOK || !typ.SameNodeOrAcyclicEqual(got, raw) {
			return got, true
		}
		return nil, false
	default:
		return nil, false
	}
}

func annotatedIdentifierType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if result == nil {
		return nil, false
	}
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return nil, false
	}
	declared, ok := newExpressionTyper(result, resolver).typeOf(expr)
	if !ok {
		return nil, false
	}
	declared = transparentComparableType(result, declared)
	if typ.IsAny(declared) || typ.IsUnknown(declared) {
		return declared, true
	}
	path, ok := result.ExpressionPath(ident)
	if !ok || path.Symbol == 0 || len(path.Segments) != 0 {
		return nil, false
	}
	value, ok := result.SymbolValueAtBoundary(point, path.Symbol)
	if !ok {
		return nil, false
	}
	return readmodel.New(result).RefineDeclaredType(declared, value)
}

func untrustedAnnotatedIdentifierType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	if result == nil {
		return nil, false
	}
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return nil, false
	}
	path, ok := result.ExpressionPath(ident)
	if !ok || path.Symbol == 0 || len(path.Segments) != 0 {
		return nil, false
	}
	annotation, ok := result.SymbolTypeAnnotation(path.Symbol)
	if !ok {
		return nil, false
	}
	declared, ok := lowerType(annotation, resolver)
	if !ok || (!typ.IsAny(declared) && !typ.IsUnknown(declared)) {
		return nil, false
	}
	return declared, true
}

func refineAssignmentSourceType(result *body.Result, point cfg.Point, expr ast.Expr, got typ.Type) typ.Type {
	if got == nil {
		return got
	}
	if result == nil {
		return got
	}
	if _, ok := result.ExpressionPath(expr); !ok {
		return got
	}
	value, ok := result.ExpressionValueAtBoundary(point, expr)
	if !ok {
		return got
	}
	refined, ok := readmodel.New(result).RefineDeclaredType(got, value)
	if !ok {
		return got
	}
	if !topLikeType(got) && !subtype.IsSubtype(refined, got) {
		return got
	}
	return refined
}

func expectedTypeAtSegments(root typ.Type, segments []segment.Segment) (typ.Type, bool) {
	current := root
	for _, seg := range segments {
		next, ok := expectedSegmentType(current, seg)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, current != nil
}

func expectedSegmentType(t typ.Type, seg segment.Segment) (typ.Type, bool) {
	t = transparentExpectedType(t)
	switch tt := t.(type) {
	case *typ.Optional:
		return expectedSegmentType(tt.Inner, seg)
	case *typ.Union:
		var matches []typ.Type
		for _, member := range tt.Members {
			if next, ok := expectedSegmentType(member, seg); ok {
				matches = append(matches, next)
			}
		}
		if len(matches) == 0 {
			return nil, false
		}
		return normalize.UnionForEvidence(matches...), true
	case *typ.Intersection:
		var matches []typ.Type
		for _, member := range tt.Members {
			if next, ok := expectedSegmentType(member, seg); ok {
				matches = append(matches, next)
			}
		}
		if len(matches) == 0 {
			return nil, false
		}
		return normalize.IntersectionForMeet(matches...), true
	case *typ.Array:
		if seg.Kind != segment.SegmentIndexInt {
			return nil, false
		}
		return tt.Element, tt.Element != nil
	case *typ.Tuple:
		if seg.Kind != segment.SegmentIndexInt || seg.Index <= 0 || seg.Index > len(tt.Elements) {
			return nil, false
		}
		elem := tt.Elements[seg.Index-1]
		return elem, elem != nil
	case *typ.Record:
		return expectedRecordSegmentType(tt, seg)
	case *typ.Map:
		if key, ok := luatypeprojection.SegmentKeyType(seg); ok && subtype.IsSubtype(key, tt.Key) {
			return tt.Value, tt.Value != nil
		}
	case *typ.ReadonlyMap:
		if key, ok := luatypeprojection.SegmentKeyType(seg); ok && subtype.IsSubtype(key, tt.Key) {
			return tt.Value, tt.Value != nil
		}
	}
	return nil, false
}

func expectedRecordSegmentType(record *typ.Record, seg segment.Segment) (typ.Type, bool) {
	if record == nil {
		return nil, false
	}
	switch seg.Kind {
	case segment.SegmentField:
		if field := record.GetField(seg.Name); field != nil {
			return field.Type, field.Type != nil
		}
	case segment.SegmentIndexString:
		if member := record.GetStaticStringIndex(seg.Name); member != nil {
			return member.Type, member.Type != nil
		}
	case segment.SegmentIndexInt:
		if member := record.GetStaticIntIndex(int64(seg.Index)); member != nil {
			return member.Type, member.Type != nil
		}
	}
	if !record.HasMapComponent() {
		return nil, false
	}
	key, ok := luatypeprojection.SegmentKeyType(seg)
	if !ok || !subtype.IsSubtype(key, record.MapKey) {
		return nil, false
	}
	return record.MapValue, record.MapValue != nil
}

func missingRequiredRecordField(want typ.Type, fact semantics.ObjectLiteralFact) (typ.Field, bool) {
	record, ok := closedRecord(want)
	if !ok {
		return typ.Field{}, false
	}
	present := make(map[string]struct{}, len(fact.Entries))
	for _, entry := range fact.Entries {
		if len(entry.Suffix.Segments) != 1 {
			continue
		}
		seg := entry.Suffix.Segments[0]
		if seg.Kind == segment.SegmentField && seg.Name != "" {
			present[seg.Name] = struct{}{}
		}
	}
	for _, field := range record.Fields {
		if field.Optional {
			continue
		}
		if _, ok := present[field.Name]; ok {
			continue
		}
		return field, true
	}
	return typ.Field{}, false
}

func closedRecord(t typ.Type) (*typ.Record, bool) {
	record, ok := transparentExpectedType(t).(*typ.Record)
	if !ok || record == nil || record.Open {
		return nil, false
	}
	return record, true
}

func transparentExpectedType(t typ.Type) typ.Type {
	for depth := 0; depth <= typ.DefaultRecursionDepth; depth++ {
		switch tt := t.(type) {
		case *typ.Annotated:
			if tt.Inner == nil || tt.Inner == t {
				return typ.Unknown
			}
			t = tt.Inner
		case *typ.Alias:
			next := tt.UnaliasedTarget()
			if next == nil || next == t {
				return next
			}
			t = next
		case *typ.Recursive:
			if tt.Body == nil || tt.Body == t {
				return t
			}
			t = tt.Body
		case *typ.Instantiated:
			next := subst.ExpandInstantiated(tt)
			if next == nil || next == t {
				return t
			}
			t = next
		default:
			return t
		}
	}
	return t
}
