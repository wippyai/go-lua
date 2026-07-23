package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// PathStoreTransaction is the immutable, syntax-free N4 path-store program.
// PathAssignment and PathStaticMemberWrite are planned together because the
// assignment's heap invalidation rule consults the sibling static write. An
// object literal is an applied-assignment sidecar and therefore remains inside
// the same transaction. A same-point RootAssignment defers its call-presence
// publication to this assignment-group barrier. Covariant exposure remains the
// whole-point N6 finalizer, while its proof-suppression policy is consulted by
// the assignment core through the frozen Facts authority.
type PathStoreTransaction struct {
	point               cfg.Point
	assignment          factflow.PathAssignment
	static              factflow.PathStaticMemberWrite
	hasAssign           bool
	hasStatic           bool
	expressionPaths     map[factflow.ExprRef]pathdom.Path
	objects             map[factflow.ExprRef]factflow.ObjectLiteralView
	exposures           []factflow.CovariantExposure
	staticHasAnnotation bool
	deferredPresence    bool
}

func PlanPathStoreTransaction(facts factflow.Facts, point cfg.Point) (PathStoreTransaction, bool) {
	return planPathStoreTransaction(facts, nil, point)
}

// PlanPathStoreTransactionWithGraph plans a PathStoreTransaction for callers
// that already hold the point's CFG.
func PlanPathStoreTransactionWithGraph(facts factflow.Facts, graph cfg.Graph, point cfg.Point) (PathStoreTransaction, bool) {
	return planPathStoreTransaction(facts, graph, point)
}

func planPathStoreTransaction(facts factflow.Facts, graph cfg.Graph, point cfg.Point) (PathStoreTransaction, bool) {
	assignment, hasAssign := facts.PathAssignment(point)
	static, hasStatic := facts.PathStaticMemberWrite(point)
	if !hasAssign && !hasStatic {
		return PathStoreTransaction{}, false
	}
	if hasAssign && (assignment.TargetPathRef().IsEmpty() || len(assignment.TargetPathRef().Segments) == 0 || !assignment.Source().Valid()) {
		return PathStoreTransaction{}, false
	}
	if hasStatic && (static.TargetPathRef().IsEmpty() || len(static.TargetPathRef().Segments) == 0 || !static.Source().Valid()) {
		return PathStoreTransaction{}, false
	}
	t := PathStoreTransaction{point: point, assignment: assignment, static: static, hasAssign: hasAssign, hasStatic: hasStatic}
	t.exposures = facts.CovariantExposures(point)
	if ordinary, ok := facts.OrdinaryAssignment(point); ok {
		_, hasAnnotation := ordinary.DeclaredAnnotationValue()
		t.staticHasAnnotation = ordinary.DeclaredValueContracts() || ordinary.DeclaredValueOverlays() || hasAnnotation
	}
	if root, ok := facts.RootAssignment(point); ok {
		source := root.Source()
		t.deferredPresence = source.Kind == factflow.ValueSourceCall && source.HasCallPoint
	}
	seen := make(map[factflow.ValueSource]bool)
	validObjects := true
	activeObjects := make(map[factflow.ExprRef]bool)
	doneObjects := make(map[factflow.ExprRef]bool)
	var freezeSource func(factflow.ValueSource)
	freezeSource = func(source factflow.ValueSource) {
		if !validObjects || !source.Valid() {
			return
		}
		firstSourceVisit := !seen[source]
		seen[source] = true
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return
		}
		if firstSourceVisit {
			if p, ok := facts.ExpressionPathRef(source.ExprRef); ok {
				if t.expressionPaths == nil {
					t.expressionPaths = make(map[factflow.ExprRef]pathdom.Path)
				}
				t.expressionPaths[source.ExprRef] = p
			}
			if dynamic, ok := facts.DynamicIndexExpression(source.ExprRef); ok {
				freezeSource(dynamic.KeySource())
			}
		}
		literal, ok := facts.ObjectLiteralView(source.ExprRef)
		if !ok {
			return
		}
		if _, identified := literal.Identity(); !identified || activeObjects[source.ExprRef] {
			validObjects = false
			return
		}
		if doneObjects[source.ExprRef] {
			return
		}
		activeObjects[source.ExprRef] = true
		defer delete(activeObjects, source.ExprRef)
		if t.objects == nil {
			t.objects = make(map[factflow.ExprRef]factflow.ObjectLiteralView)
		}
		literal = freezeObjectLiteralView(literal)
		t.objects[source.ExprRef] = literal
		literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
			freezeSource(entry.Source())
			return true
		})
		if list, ok := literal.ListElementSource(); ok {
			freezeSource(list)
		}
		if validObjects {
			doneObjects[source.ExprRef] = true
		}
	}
	if hasAssign {
		freezeSource(assignment.Source())
	}
	if hasStatic {
		freezeSource(static.Source())
	}
	if !validObjects {
		return PathStoreTransaction{}, false
	}
	return t, true
}

func freezeObjectLiteralView(view factflow.ObjectLiteralView) factflow.ObjectLiteralView {
	entries := make([]factflow.ObjectEntry, 0, view.EntryCount())
	view.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		owned := factflow.NewObjectEntryWithMetadata(entry.Suffix(), entry.Source(), entry.ValueSpan(), entry.ValueLabel())
		if expected, ok := entry.Expected(); ok {
			owned = owned.WithExpected(expected)
		}
		entries = append(entries, owned)
		return true
	})
	owned := factflow.NewObjectLiteral(entries)
	if source, ok := view.ListElementSource(); ok {
		owned = owned.WithListElementSource(source)
	}
	if view.StaticStringKeysComplete() {
		owned = owned.WithStaticStringKeysComplete()
	}
	if expected, ok := view.Expected(); ok {
		owned = owned.WithExpected(expected)
	}
	if id, ok := view.Identity(); ok {
		owned = owned.WithIdentity(id)
	}
	if span, ok := view.Span(); ok {
		owned = owned.WithSpan(span)
	}
	if id, ok := view.ExpressionID(); ok {
		owned = owned.WithExpressionID(id)
	}
	return owned.View()
}

func (t PathStoreTransaction) Point() cfg.Point           { return t.point }
func (t PathStoreTransaction) HasPathAssignment() bool    { return t.hasAssign }
func (t PathStoreTransaction) HasStaticMemberWrite() bool { return t.hasStatic }
func (t PathStoreTransaction) StaticHasAnnotation() bool  { return t.staticHasAnnotation }

// Valid checks the complete immutable transaction payload, including the
// object/covariant dependency variants interpreted by the assignment core.
func (t PathStoreTransaction) Valid(reg *axis.Registry) bool {
	if reg == nil || !t.hasAssign && !t.hasStatic {
		return false
	}
	if t.hasAssign && (t.assignment.TargetPathRef().IsEmpty() || len(t.assignment.TargetPathRef().Segments) == 0 || !t.assignment.Source().Valid()) {
		return false
	}
	if t.hasStatic && (t.static.TargetPathRef().IsEmpty() || len(t.static.TargetPathRef().Segments) == 0 || !t.static.Source().Valid()) {
		return false
	}
	if t.HasObjectLiteralSidecar() {
		valid := true
		seen := make(map[factflow.ExprRef]bool)
		var validateObject func(factflow.ValueSource)
		validateObject = func(source factflow.ValueSource) {
			if !valid || source.Kind != factflow.ValueSourceExpression || !source.HasExpr || seen[source.ExprRef] {
				return
			}
			view, ok := t.objectLiteral(source.ExprRef)
			if !ok {
				return
			}
			seen[source.ExprRef] = true
			view.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
				if entry.SuffixSegmentCount() == 0 || !entry.Source().Valid() {
					valid = false
					return false
				}
				if expected, ok := entry.Expected(); ok && !product.BelongsToRegistry(reg, expected) {
					valid = false
					return false
				}
				validateObject(entry.Source())
				return valid
			})
		}
		validateObject(t.assignment.Source())
		if !valid {
			return false
		}
	}
	for _, exposure := range t.exposures {
		if exposure.SourcePath().IsEmpty() || !product.BelongsToRegistry(reg, exposure.WideValue()) {
			return false
		}
	}
	return true
}

func (t PathStoreTransaction) AssignmentTarget() (pathdom.Path, bool) {
	if !t.hasAssign {
		return pathdom.Path{}, false
	}
	return t.assignment.TargetPath(), true
}
func (t PathStoreTransaction) AssignmentSource() (factflow.ValueSource, bool) {
	if !t.hasAssign {
		return factflow.ValueSource{}, false
	}
	return t.assignment.Source(), true
}
func (t PathStoreTransaction) StaticTarget() (pathdom.Path, bool) {
	if !t.hasStatic {
		return pathdom.Path{}, false
	}
	return t.static.TargetPath(), true
}
func (t PathStoreTransaction) StaticSource() (factflow.ValueSource, bool) {
	if !t.hasStatic {
		return factflow.ValueSource{}, false
	}
	return t.static.Source(), true
}
func (t PathStoreTransaction) HasObjectLiteralSidecar() bool {
	if !t.hasAssign {
		return false
	}
	source := t.assignment.Source()
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return false
	}
	_, ok := t.objectLiteral(source.ExprRef)
	return ok
}

// ObjectLiteralForSource exposes the immutable object view owned by this
// transaction. Consumers compile this frozen payload; they never re-query
// Facts or rediscover object structure during specialization.
func (t PathStoreTransaction) ObjectLiteralForSource(source factflow.ValueSource) (factflow.ObjectLiteralView, bool) {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return factflow.ObjectLiteralView{}, false
	}
	return t.objectLiteral(source.ExprRef)
}
func (t PathStoreTransaction) HasCovariantProofPolicy() bool {
	return len(t.exposures) != 0
}

// HasAssignmentGroupPresenceStep reports the RootAssignment-owned publication
// barrier which is scheduled between applied-gated object materialization and
// the independent static-member write.
func (t PathStoreTransaction) HasAssignmentGroupPresenceStep() bool {
	return t.deferredPresence
}

func (t PathStoreTransaction) objectLiteral(ref factflow.ExprRef) (factflow.ObjectLiteralView, bool) {
	literal, ok := t.objects[ref]
	return literal, ok
}

func (t PathStoreTransaction) sourcePath(resolver *visibility.Resolver, source factflow.ValueSource) (pathdom.Path, bool) {
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		path, ok := t.expressionPaths[source.ExprRef]
		return path, ok
	}
	if source.Kind != factflow.ValueSourcePath || source.PathKey == "" || resolver == nil || resolver.KeySpace() == nil {
		return pathdom.Path{}, false
	}
	if sym, segments, ok := pathaddr.ParseSymbolPathKey(source.PathKey); ok {
		return pathdom.Path{Symbol: sym, Segments: segments}, true
	}
	key, ok := resolver.KeySpace().FromStateKey(source.PathKey)
	if !ok || key.Sym == 0 {
		return pathdom.Path{}, false
	}
	return pathdom.Path{Symbol: key.Sym, Segments: resolver.KeySpace().Segments(key)}, true
}

func (t PathStoreTransaction) suppressesPathProof(resolver *visibility.Resolver, source factflow.ValueSource) bool {
	path, ok := t.sourcePath(resolver, source)
	if !ok || path.Symbol == 0 || len(path.Segments) != 0 {
		return false
	}
	for _, exposure := range t.exposures {
		exposed := exposure.SourcePath()
		if exposure.Kind() == factflow.CovariantExposureRecord && exposed.Symbol == path.Symbol && len(exposed.Segments) == 0 {
			return true
		}
	}
	return false
}

// SuppressesPathProof reports the frozen covariant record policy for sources
// whose structural path was captured at Plan time.
func (t PathStoreTransaction) SuppressesPathProof(source factflow.ValueSource) bool {
	return t.suppressesPathProof(nil, source)
}

// SourcePath reports the structural source path captured when the immutable
// transaction was planned. It never consults Facts during execution.
func (t PathStoreTransaction) SourcePath(source factflow.ValueSource) (pathdom.Path, bool) {
	return t.sourcePath(nil, source)
}
