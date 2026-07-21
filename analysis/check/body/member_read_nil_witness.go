package body

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// WithMemberReadNilWitness adds a nil witness to value when expr is a member
// read whose solved evidence can still miss at point. Body owns this decision
// because it combines expression paths, declared types, heap/path proofs, and
// branch evidence from the solved body.
func (r *Result) WithMemberReadNilWitness(point cfg.Point, expr ast.Expr, value product.Value) product.Value {
	// distrustExactnessProofs covers every reason a point-local exactness fact
	// can be stale: a synchronously invoked closure capture, or a write through
	// a differently-rooted alias of the same object that the point-local
	// path/static-member lattice does not always learn about.
	distrustExactnessProofs := r.memberReadHasPriorStructuralMutation(point, expr) ||
		r.memberReadHasCrossSymbolAliasWriteReachable(point, expr)
	if r == nil || r.Registry() == nil || expr == nil ||
		(!distrustExactnessProofs && !r.MemberReadCanMissOrDeclaredNilable(point, expr)) ||
		(!distrustExactnessProofs &&
			(r.memberReadValueAlreadyProvenPresent(point, expr, value) ||
				r.ExpressionReadProvenPresentBeforeBoundary(point, expr) ||
				r.memberReadHasExactPathProof(point, expr) ||
				r.memberReadHasExactHeapProof(point, expr) ||
				r.memberReadHasLengthFloorProof(point, expr) ||
				r.memberReadHasLiteralDeclarationProof(point, expr))) {
		return value
	}
	got, ok := r.ValueTypeWithPresence(value)
	if !ok || got == nil || typ.IsAny(got) || typ.IsUnknown(got) {
		if declared, declaredOK := r.DeclaredExpressionTypeAt(point, expr); declaredOK &&
			declared != nil &&
			!typ.IsAny(declared) &&
			!typ.IsUnknown(declared) {
			value = typevalue.FromType(r.Registry(), declared)
			if typevalue.TypeIncludesNil(declared) {
				return product.WithPresence(r.Registry(), value, presence.Maybe())
			}
			got = declared
		} else {
			return value
		}
	}
	if typevalue.TypeIncludesNil(got) {
		return value
	}
	value = product.WithPresence(r.Registry(), value, presence.Maybe())
	return typevalue.WithWitness(r.Registry(), value, normalize.Optional(got))
}

func (r *Result) memberReadValueAlreadyProvenPresent(point cfg.Point, expr ast.Expr, value product.Value) bool {
	if r == nil || r.Registry() == nil || !presence.Equal(product.PresenceOf(value), presence.Present()) {
		return false
	}
	if r.memberReadReceiverMayBeNil(point, expr) {
		return false
	}
	p, ok := r.ExpressionPath(expr)
	if !ok || p.IsEmpty() {
		return false
	}
	solved, ok := r.PathValueBeforeBoundary(point, p)
	if !ok {
		return false
	}
	return presence.Equal(product.PresenceOf(solved), presence.Present())
}

// MemberReadCanMissOrDeclaredNilable reports whether expr is a member read that
// may be absent from its container or whose declaration is explicitly nilable.
func (r *Result) MemberReadCanMissOrDeclaredNilable(point cfg.Point, expr ast.Expr) bool {
	if r.MemberReadCanMiss(point, expr) {
		return true
	}
	if _, ok := expr.(*ast.AttrGetExpr); !ok {
		return false
	}
	if current, ok := r.ExpressionTypeBeforeBoundary(point, expr); ok &&
		current != nil &&
		!typ.IsAny(current) &&
		!typ.IsUnknown(current) &&
		!typevalue.TypeIncludesNil(current) {
		return false
	}
	declared, ok := r.DeclaredExpressionTypeAt(point, expr)
	return ok && typevalue.TypeIncludesNil(declared)
}

// memberReadHasPriorStructuralMutation prevents a stale heap/path member fact
// from proving a later read present after an opaque call can synchronously
// invoke a supplied closure and mutate the table through its capture. Ordinary
// prior writes remain governed by their existing point-sensitive facts.
func (r *Result) memberReadHasPriorStructuralMutation(point cfg.Point, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.Object == nil || r == nil || r.registry == nil {
		return false
	}
	receiver, ok := r.ExpressionValueBeforeBoundary(point, attr.Object)
	if !ok {
		return false
	}
	id, ok := identityvalue.ExactID(r.registry, receiver)
	if !ok {
		return false
	}
	graph := r.Graph()
	if graph == nil {
		return false
	}
	entry := graph.Entry()
	for _, candidate := range graph.RPO() {
		if candidate == point ||
			!r.PointCanReach(entry, candidate) ||
			!r.PointCanReach(candidate, point) {
			continue
		}
		if r.opaqueCallMaySynchronouslyInvokeCapturedClosure(candidate) &&
			r.callArgumentGraphCapturesIdentity(candidate, id) {
			return true
		}
	}
	return false
}

// memberReadHasCrossSymbolAliasWriteReachable reports whether a write reachable
// before point targets the same field suffix as expr's receiver through a
// different root local. Point-local path/static-member facts are keyed by
// syntactic local path; a plain "local alias = box" declaration does not
// always record a path-equality proof between alias and box, so a write
// through the differently-named alias can leave a stale presence fact for
// expr's own path. This re-derives aliasing from the same runtime-identity
// mechanism dominance-based guard queries already use (assignmentPathMayTouch
// RecoveredPath / descendantInvalidationMayTouchRecoveredPath), restricted to
// writes through a different root symbol: a direct rewrite of the same local
// is already reflected correctly by the forward state and must not be
// invalidated here, or every ordinary field reassignment would spuriously
// lose its narrowing.
func (r *Result) memberReadHasCrossSymbolAliasWriteReachable(point cfg.Point, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.Object == nil {
		return false
	}
	suffix, ok := memberReadSuffix(attr)
	if !ok {
		return false
	}
	objectPath, ok := r.ExpressionPath(attr.Object)
	if !ok || objectPath.IsEmpty() || objectPath.Symbol == 0 {
		return false
	}
	target := objectPath.AppendSegments(suffix)
	graph := r.Graph()
	if graph == nil {
		return false
	}
	entry := graph.Entry()
	for _, candidate := range graph.RPO() {
		if candidate == point || !r.PointCanReach(entry, candidate) || !r.PointCanReach(candidate, point) {
			continue
		}
		if assignment, ok := r.OrdinaryAssignment(candidate); ok && assignment.HasPath &&
			assignment.Path.Symbol != 0 && assignment.Path.Symbol != target.Symbol &&
			r.assignmentPathMayTouchRecoveredPath(candidate, assignment.Path, target) {
			return true
		}
		if invalidation, ok := r.PathDescendantInvalidation(candidate); ok {
			container := invalidation.ContainerPath()
			if container.Symbol != 0 && container.Symbol != target.Symbol &&
				r.descendantInvalidationMayTouchRecoveredPath(candidate, invalidation, target) {
				return true
			}
		}
	}
	return false
}

// MemberReadCanMiss reports whether expr's member access has a nilable result
// under the best container/key type known before point.
func (r *Result) MemberReadCanMiss(point cfg.Point, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.Object == nil || attr.Key == nil {
		return false
	}
	if receiverValue, valueOK := r.ExpressionValueBeforeBoundary(point, attr.Object); valueOK {
		if receiverType, typeOK := r.ValueTypeWithPresence(receiverValue); typeOK &&
			typevalue.TypeIncludesNil(receiverType) &&
			!r.ExpressionReadProvenPresentBeforeBoundary(point, attr.Object) {
			return true
		}
	}
	container, ok := r.ExpressionTypeBeforeBoundary(point, attr.Object)
	if !ok {
		container, ok = r.DeclaredExpressionTypeAt(point, attr.Object)
	}
	if !ok || container == nil {
		return false
	}
	if r.ExpressionReadProvenPresentBeforeBoundary(point, attr.Object) {
		if withoutNil := ProjectionWithoutNil(container); withoutNil != nil && !typ.IsNever(withoutNil) {
			container = withoutNil
		}
	}
	var key typ.Type
	if attr.KeySyntax != ast.AttrKeyIndex {
		name := ast.KeyName(attr.Key)
		if name == "" {
			return false
		}
		key = typ.LiteralString(name)
	} else {
		key, ok = r.ExpressionTypeBeforeBoundary(point, attr.Key)
		if !ok {
			key, _ = LiteralExpressionType(attr.Key)
		}
	}
	if key == nil {
		return false
	}
	got, ok := access.RuntimeIndex(container, key)
	return ok && typevalue.TypeIncludesNil(got)
}

func (r *Result) memberReadHasExactPathProof(point cfg.Point, expr ast.Expr) bool {
	if r == nil {
		return false
	}
	if r.memberReadReceiverMayBeNil(point, expr) {
		return false
	}
	p, ok := r.ExpressionPath(expr)
	if !ok || p.IsEmpty() || p.Symbol == 0 || len(p.Segments) == 0 {
		return false
	}
	st, ok := r.StateAt(point)
	if !ok {
		return false
	}
	key, ok := valueSourcePathKeyspaceKey(r.visibility, point, p)
	if !ok {
		return false
	}
	return readCanonicalPathStaticMember(st, r.KeySpace(), key)
}

func (r *Result) memberReadReceiverMayBeNil(point cfg.Point, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.Object == nil || r.ExpressionReadProvenPresentBeforeBoundary(point, attr.Object) {
		return false
	}
	if receiverValue, valueOK := r.ExpressionValueBeforeBoundary(point, attr.Object); valueOK {
		if receiverType, typeOK := r.ValueTypeWithPresence(receiverValue); typeOK &&
			typevalue.TypeIncludesNil(receiverType) {
			return true
		}
	}
	if receiver, receiverOK := r.ExpressionTypeBeforeBoundary(point, attr.Object); receiverOK &&
		typevalue.TypeIncludesNil(receiver) {
		return true
	}
	receiverPath, pathOK := r.ExpressionPath(attr.Object)
	if !pathOK || receiverPath.IsEmpty() {
		return false
	}
	receiverValue, valueOK := r.PathValueBeforeBoundary(point, receiverPath)
	if !valueOK {
		return false
	}
	receiverType, typeOK := r.ValueTypeWithPresence(receiverValue)
	return typeOK && typevalue.TypeIncludesNil(receiverType)
}

func (r *Result) memberReadHasExactHeapProof(point cfg.Point, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.Object == nil || attr.Key == nil || r == nil || r.Registry() == nil {
		return false
	}
	suffix, ok := memberReadSuffix(attr)
	if !ok {
		return false
	}
	object, ok := r.ExpressionValueBeforeBoundary(point, attr.Object)
	if !ok {
		return false
	}
	id, ok := product.Get(r.Registry(), object, identity.Key).ID()
	if !ok {
		return false
	}
	st, ok := r.StateAt(point)
	if !ok {
		return false
	}
	memberKey, ok := heapidentity.StaticMemberSuffixKey(r.KeySpace(), suffix)
	if !ok {
		return false
	}
	table := st.ReadHeapTableObject(r.Registry(), id)
	if member, ok := table.StaticMember(memberKey); ok && presence.Equal(product.PresenceOf(member), presence.Present()) {
		return true
	}
	if canonical, ok := heapidentity.FieldCanonicalStaticMemberSuffixKey(r.KeySpace(), suffix); ok {
		if member, ok := table.StaticMember(canonical); ok && presence.Equal(product.PresenceOf(member), presence.Present()) {
			return true
		}
	}
	return false
}

func (r *Result) memberReadHasLengthFloorProof(point cfg.Point, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.Object == nil || attr.Key == nil || attr.KeySyntax != ast.AttrKeyIndex || r == nil {
		return false
	}
	key, ok := attr.Key.(*ast.NumberExpr)
	if !ok || strings.ContainsAny(key.Value, ".eE") {
		return false
	}
	index, err := strconv.ParseInt(key.Value, 10, 64)
	if err != nil || index < 1 {
		return false
	}
	arrayPath, ok := r.ExpressionPath(attr.Object)
	if !ok || arrayPath.IsEmpty() {
		return false
	}
	floor, ok := r.LengthFloorAtBoundary(point, arrayPath)
	return ok && floor >= index
}

func (r *Result) memberReadHasLiteralDeclarationProof(point cfg.Point, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.Object == nil || attr.Key == nil || r == nil {
		return false
	}
	suffix, ok := memberReadSuffix(attr)
	if !ok {
		return false
	}
	objectPath, ok := r.ExpressionPath(attr.Object)
	if !ok || objectPath.IsEmpty() || objectPath.Symbol == 0 {
		return false
	}
	declaration, ok := r.DominatingPathRootDeclarationSource(point, objectPath)
	if !ok || declaration.Source.Kind != factflow.ValueSourceExpression || !declaration.Source.HasExpr {
		return false
	}
	if r.pathShapeInvalidatedAfterAssignment(declaration.Point, point, objectPath.AppendSegments(suffix)) {
		return false
	}
	literal, ok := r.ObjectLiteralView(declaration.Source.ExprRef)
	if !ok {
		return false
	}
	found := false
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		if pathSegmentsEqual(entry.SuffixSegmentsView(), suffix) {
			found = true
			return false
		}
		return true
	})
	return found
}

func pathSegmentsEqual(left, right []segment.Segment) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func memberReadSuffix(attr *ast.AttrGetExpr) ([]segment.Segment, bool) {
	if attr == nil || attr.Key == nil {
		return nil, false
	}
	if attr.KeySyntax != ast.AttrKeyIndex {
		name := ast.KeyName(attr.Key)
		if name == "" {
			return nil, false
		}
		return []segment.Segment{{Kind: segment.SegmentField, Name: name}}, true
	}
	switch key := attr.Key.(type) {
	case *ast.StringExpr:
		return []segment.Segment{{Kind: segment.SegmentIndexString, Name: key.Value}}, true
	case *ast.NumberExpr:
		if strings.ContainsAny(key.Value, ".eE") {
			return nil, false
		}
		index, err := strconv.Atoi(key.Value)
		if err != nil {
			return nil, false
		}
		return []segment.Segment{{Kind: segment.SegmentIndexInt, Index: index}}, true
	default:
		return nil, false
	}
}

func readCanonicalPathStaticMember(st statePathStaticMemberReader, ks *keyspace.KeySpace, localKey keyspace.Key) bool {
	if ks == nil || localKey.Kind == keyspace.KindInvalid {
		return false
	}
	if _, ok := st.ReadLocalPathStaticMember(localKey); ok {
		return true
	}
	if canonical, ok := ks.FieldCanonical(localKey); ok {
		if _, ok := st.ReadLocalPathStaticMember(canonical); ok {
			return true
		}
	}
	if unversioned, ok := unversionedStaticMemberKey(ks, localKey); ok {
		if _, ok := st.ReadLocalPathStaticMember(unversioned); ok {
			return true
		}
		if canonical, ok := ks.FieldCanonical(unversioned); ok {
			if _, ok := st.ReadLocalPathStaticMember(canonical); ok {
				return true
			}
		}
	}
	if stable, ok := stableStaticMemberKey(ks, localKey); ok {
		if _, ok := st.ReadLocalPathStaticMember(stable); ok {
			return true
		}
		if canonical, ok := ks.FieldCanonical(stable); ok {
			if _, ok := st.ReadLocalPathStaticMember(canonical); ok {
				return true
			}
		}
	}
	return false
}

type statePathStaticMemberReader interface {
	ReadLocalPathStaticMember(keyspace.Key) (product.Value, bool)
}

func unversionedStaticMemberKey(ks *keyspace.KeySpace, localKey keyspace.Key) (keyspace.Key, bool) {
	if ks == nil || localKey.Kind != keyspace.KindResolverSym || localKey.Sym == 0 {
		return keyspace.Key{}, false
	}
	segments, ok := ks.SegmentsView(localKey)
	if !ok {
		return keyspace.Key{}, false
	}
	return ks.LookupResolverKey(localKey.Sym, 0, segments)
}

func stableStaticMemberKey(ks *keyspace.KeySpace, localKey keyspace.Key) (keyspace.Key, bool) {
	if ks == nil || localKey.Kind != keyspace.KindResolverSym || localKey.Sym == 0 {
		return keyspace.Key{}, false
	}
	segments, ok := ks.SegmentsView(localKey)
	if !ok {
		return keyspace.Key{}, false
	}
	return ks.FromStableSymbol(localKey.Sym, segments)
}
