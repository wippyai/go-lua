package observation

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/provenance"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// AssignmentSourceType projects the value read by an assignment source. If the
// source references the target symbol, reads use the point-entry state so
// diagnostics do not inspect the value after the assignment has overwritten it.
func (p Projector) AssignmentSourceType(source ast.Expr, point cfg.Point, expected typ.Type, targetSym cfg.SymbolID) typ.Type {
	if source == nil {
		return nil
	}
	// A stored source type that still carries a type parameter is a generic call
	// synthesized bottom-up without its expected return, so the parameter was never
	// bound. When an expected type is available, re-synthesize the source with it so
	// bidirectional inference instantiates the parameter (e.g. registry.get<T>(): T?
	// assigned to a number? annotation resolves T to number).
	if expected != nil {
		if t, ok := p.assignmentSourceProductType(point, targetSym); ok && !typ.ContainsTypeParam(t) {
			return p.admitGradualAssignmentSource(p.refineAssignmentSourceIndexRead(t, source, point), source, point, expected)
		}
		if t, ok := p.assignmentPathSourceType(source, point, targetSym); ok && !typ.ContainsTypeParam(t) {
			return p.admitGradualAssignmentSource(t, source, point, expected)
		}
		return p.assignmentSourceProjector(source, targetSym).TypeOfWithExpected(source, point, expected)
	}
	if t, ok := p.assignmentSourceProductType(point, targetSym); ok {
		return p.refineAssignmentSourceIndexRead(t, source, point)
	}
	if t, ok := p.assignmentPathSourceType(source, point, targetSym); ok {
		return t
	}
	return p.assignmentSourceProjector(source, targetSym).TypeOfWithExpected(source, point, expected)
}

// admitGradualAssignmentSource applies gradual-typing's consistency to an
// assignment source observed as the gradual top `any`, mirroring the return
// boundary's coerceGradualToExpected: such a value is consistent with every
// type, so against a concrete annotation it observes as that annotation. It is
// gated by sourceAnyIsGradualTop so a declared-`any` symbol read keeps its
// strict any->concrete rejection.
func (p Projector) admitGradualAssignmentSource(t typ.Type, source ast.Expr, point cfg.Point, expected typ.Type) typ.Type {
	if zNoGradualBoundary() {
		return t
	}
	if !typ.IsAny(t) || !p.sourceAnyIsGradualTop(source, point) {
		return t
	}
	return p.coerceGradualToExpected(t, expected)
}

// sourceAnyIsGradualTop reports whether a source observed as `any` is the
// gradual top admissible against a concrete annotation, rather than an `any` the
// boundary check must still flag.
//
// Two disjoint shapes are the gradual top:
//
//   - A read whose PATH ROOT is an unannotated parameter. An unannotated parameter
//     is the gradual top by inference: the function asserts no type for it, so the
//     parameter and every field/index projection of it are consistent with any
//     concrete target (a bare `url` parameter, or `http_response.body` read off an
//     unannotated `http_response`). This is gradual consistency, not a declared
//     `any`: a parameter explicitly annotated `any` is an opt-in to the strict
//     dynamic contract and is excluded by isUnannotatedParamSymbol.
//   - A field/index read OFF A TYPED CONTAINER whose projected field/element is
//     genuinely `any`: a value the inference could type the container of but not
//     the member (a `{ data_func: any }` record built from a `{[string]: any}`
//     dynamic map). Gradual consistency admits it against a concrete expected type.
//
// All other `any` reads keep strict rejection. A read whose CONTAINER is itself
// `any`/`unknown` (and whose root is NOT an unannotated parameter) is not the
// gradual top: the flow cannot type the object at all, so the projected `any` is
// an unproven guess (a wrapper whose return widened to `any` on a non-dominating
// path, or a declared-`any` record field read into a local). A bare symbol read
// whose symbol is a declared `any` (an annotated parameter, or a local typed
// `any`) keeps its any->concrete contract (the cast-guard soundness contract).
func (p Projector) sourceAnyIsGradualTop(source ast.Expr, point cfg.Point) bool {
	path := p.pathOfExpr(source, point)
	if path.IsEmpty() || path.Symbol == 0 {
		return false
	}
	if p.isUnannotatedParamSymbol(path.Symbol) {
		return true
	}
	if len(path.Segments) == 0 {
		return false
	}
	attr, ok := source.(*ast.AttrGetExpr)
	if !ok || attr == nil {
		return false
	}
	obj := unwrap.Alias(p.TypeOf(attr.Object, point))
	return obj != nil && !obj.Kind().IsPlaceholder()
}

// AssignmentSourceTableCheck validates a table literal through the same
// assignment-source boundary as AssignmentSourceType. Self-referential writes
// therefore observe the RHS in the point-entry state for both value projection
// and contextual table compatibility.
func (p Projector) AssignmentSourceTableCheck(table *ast.TableExpr, point cfg.Point, expected typ.Type, targetSym cfg.SymbolID) TableCheckResult {
	return p.assignmentSourceProjector(table, targetSym).CheckTable(table, point, expected)
}

// refineAssignmentSourceIndexRead applies the solved index-read proof to a
// flow-derived assignment-source value. The flow product algebra resolves
// data[i] to the element union (with nil for out-of-range), but it does not
// consult the numeric-interval / length proofs that prove the index in range.
// Routing the product through the same proof as a directly observed read makes
// loop-variable reads (for and while alike) honor those proofs uniformly.
func (p Projector) refineAssignmentSourceIndexRead(t typ.Type, source ast.Expr, point cfg.Point) typ.Type {
	attr, ok := source.(*ast.AttrGetExpr)
	if !ok || attr == nil {
		return t
	}
	return p.applyIndexReadProof(t, p.TypeOf(attr.Object, point), attr.Object, attr.Key, point)
}

func (p Projector) assignmentSourceProjector(source ast.Expr, targetSym cfg.SymbolID) Projector {
	if targetSym != 0 && p.exprReferencesSymbol(source, targetSym) {
		return p.WithPreStateReads()
	}
	return p
}

func (p Projector) assignmentSourceProductType(point cfg.Point, targetSym cfg.SymbolID) (typ.Type, bool) {
	if p.cfg.Inputs == nil || p.cfg.Solution == nil || targetSym == 0 {
		return nil, false
	}
	for _, assign := range p.cfg.Inputs.Assignments {
		if assign.Point != point || assign.TargetPath.Symbol != targetSym {
			continue
		}
		if assign.Source.Kind == flow.AssignmentSourceStatic && assign.Source.ProjectionKind == flow.AssignmentSourceProjectionNone {
			return nil, false
		}
		t := p.cfg.Solution.AssignmentSourceValueAt(point, assign.TargetPath, assign.Source)
		if typ.IsAbsentOrUnknown(t) {
			return nil, false
		}
		return t, true
	}
	return nil, false
}

func (p Projector) assignmentPathSourceType(source ast.Expr, point cfg.Point, targetSym cfg.SymbolID) (typ.Type, bool) {
	path := p.pathOfExpr(source, point)
	if path.IsEmpty() {
		return nil, false
	}
	declared := p.pathDeclaredType(point, path)
	if p.cfg.Solution != nil {
		if targetSym != 0 && p.exprReferencesSymbol(source, targetSym) {
			if t := p.cfg.Solution.PreStateTypeAt(point, path); !typ.IsAbsentOrUnknown(t) {
				return p.reconcileObservedPath(t, declared), true
			}
		} else if t := p.cfg.Solution.NarrowedTypeAt(point, path); !typ.IsAbsentOrUnknown(t) {
			return p.reconcileObservedPath(t, declared), true
		}
	} else if t, ok := p.factsNarrowedPathType(point, path); ok {
		// No Solution: a flow whose narrowing lives in the per-point Facts (the
		// canonical flow) supplies the path's flow-refined type here, so an
		// assignment source observes the value narrowed by the branch guard rather
		// than its flow-insensitive declared type.
		return p.reconcileObservedPath(t, declared), true
	}
	if declared != nil {
		return declared, true
	}
	return nil, false
}

// factsNarrowedPathType resolves the flow-refined type of path from the per-point
// Facts, the narrowing surface a Solution-less flow (the canonical flow) exposes.
// It reads the refined base symbol type and derives through the path segments the
// same way pathDeclaredType walks the declared type, so a discriminant-narrowed
// base.field read resolves to the narrowed variant's field. Returns false when no
// sound refinement is available for the path root (then the declared type is used).
//
// The root refinement is admitted only when it is a sound narrowing of the root's
// declared type: an annotated symbol keeps a gradual (any) contract, and a refined
// type that is not a subtype of the declared type is rejected. This mirrors the
// legacy Solution.EffectiveTypeAt annotated-symbol guard so the canonical flow does
// not replace a declared type with a refinement the assignment check must still
// validate against the declaration (e.g. a value typed any must stay any at an
// assignment source so an any -> concrete write is flagged, not silently admitted).
func (p Projector) factsNarrowedPathType(point cfg.Point, path constraint.Path) (typ.Type, bool) {
	if p.cfg.Facts == nil || path.Symbol == 0 {
		return nil, false
	}
	refined := p.cfg.Facts.RefinedAt(point, path.Symbol)
	if refined.State != flow.StateResolved || refined.Type == nil || typ.IsUnknown(refined.Type) {
		return nil, false
	}
	root := p.soundRootRefinement(point, path.Symbol, refined.Type)
	if root == nil {
		return nil, false
	}
	if len(path.Segments) == 0 {
		return root, true
	}
	derived := p.derivePathSegments(root, path.Segments)
	if derived == nil {
		return nil, false
	}
	return p.refineFactsLengthIndex(point, path, root, derived), true
}

// refineFactsLengthIndex recovers a non-optional element type for an assignment
// source `arr[k]` a length proof shows is in range, the read-side counterpart of
// the path-sensitive Solution's index-read refinement for a Solution-less flow. It
// fires only for a single trailing positive literal index over the path's root
// container and consults the flow's LengthFacts proof: when the proven length lower
// bound covers the index, the same narrow law the path-sensitive flow uses
// (RefineSequenceIndex) drops the in-range read's flow-uncertainty nil. A read the
// proof cannot place in range keeps its optional element (sound).
func (p Projector) refineFactsLengthIndex(point cfg.Point, path constraint.Path, container, derived typ.Type) typ.Type {
	if len(path.Segments) != 1 {
		return derived
	}
	seg := path.Segments[0]
	if seg.Kind != constraint.SegmentIndexInt || seg.Index < 1 {
		return derived
	}
	lengthFacts, ok := p.cfg.Facts.(flow.LengthFacts)
	if !ok {
		return derived
	}
	lower, ok := lengthFacts.LengthLowerBoundAt(point, path.Symbol)
	if !ok || lower < int64(seg.Index) {
		return derived
	}
	if refined := narrow.RefineSequenceIndex(container, derived, int64(seg.Index)); refined != nil {
		return refined
	}
	return derived
}

// soundRootRefinement gates a path-root refinement against the symbol's declared
// type, mirroring the legacy Solution.EffectiveTypeAt annotated-symbol guard. For an
// unannotated symbol the refinement is the inferred value (used as is). For an
// annotated symbol:
//
//   - a gradual declared type (any/unknown) keeps its contract: a value declared
//     any must stay any at an assignment source so an any -> concrete write is
//     flagged, not silently admitted by an inferred concrete refinement;
//   - a refinement that is not a subtype of the declared type is rejected (an
//     unsound narrowing that would drop declared structure);
//
// otherwise the refinement (a sound narrowing within the declaration, e.g.
// string narrowed from string?) is admitted. Returns nil to decline the override.
func (p Projector) soundRootRefinement(point cfg.Point, sym cfg.SymbolID, refined typ.Type) typ.Type {
	declared := p.declaredSymbolType(point, sym)
	if declared == nil {
		return refined
	}
	if declared.Kind().IsPlaceholder() {
		// A gradual (any/unknown) declaration keeps its contract for an UNNARROWED
		// read: a value declared any must stay any at an assignment source so an
		// any -> concrete write is flagged, not silently admitted by an inferred
		// concrete value. But a path-sensitive type guard (a type(x) == k / T:is(x)
		// success edge) narrows the gradual value to a concrete type; that refinement
		// is sound to admit (the guard proved it). In the canonical flow the only way
		// a gradual symbol's refined value becomes a concrete strict-subtype is such a
		// guard narrowing, so a non-placeholder refinement is the guarded narrowing
		// and is admitted; a still-gradual refinement keeps the declared contract.
		if refined.Kind().IsPlaceholder() {
			return nil
		}
		return refined
	}
	if !subtype.IsSubtype(refined, declared) {
		return nil
	}
	return refined
}

// derivePathSegments walks base through the field/index segments of a path,
// returning the type the path denotes or nil when a segment does not resolve. It
// reuses the same query-core field/index resolution pathDeclaredType uses.
func (p Projector) derivePathSegments(base typ.Type, segments []constraint.Segment) typ.Type {
	current := base
	for _, segment := range segments {
		var next typ.Type
		switch segment.Kind {
		case constraint.SegmentField, constraint.SegmentIndexString:
			if p.cfg.TypeOps != nil {
				next, _ = p.cfg.TypeOps.Field(p.cfg.Ctx, current, segment.Name)
				if next == nil {
					next, _ = p.cfg.TypeOps.Index(p.cfg.Ctx, current, typ.LiteralString(segment.Name))
				}
			} else {
				next, _ = querycore.Field(current, segment.Name)
				if next == nil {
					next, _ = querycore.Index(current, typ.LiteralString(segment.Name))
				}
			}
		case constraint.SegmentIndexInt:
			key := typ.LiteralInt(int64(segment.Index))
			if p.cfg.TypeOps != nil {
				next, _ = p.cfg.TypeOps.Index(p.cfg.Ctx, current, key)
			} else {
				next, _ = querycore.Index(current, key)
			}
		default:
			return nil
		}
		if next == nil {
			if querycore.MissingFieldReadsNil(current) {
				next = typ.Nil
			} else {
				return nil
			}
		}
		current = next
	}
	return current
}

// AssignmentTargetWriteType projects the solved write-slot type for an
// assignment target.
func (p Projector) AssignmentTargetWriteType(target cfg.AssignTarget, source ast.Expr, point cfg.Point) typ.Type {
	if target.Kind != cfg.TargetIndex || target.Base == nil {
		return nil
	}
	if t := p.assignmentTargetFlowWriteType(target, source, point); t != nil {
		return t
	}
	objType := p.TypeOf(target.Base, point)
	keyType := typ.Type(nil)
	if target.Key != nil {
		keyType = p.TypeOf(target.Key, point)
	}
	if expected, ok := querycore.IndexWrite(objType, keyType); ok {
		return expected
	}
	if expected, ok := querycore.IndexWriteObligation(objType, keyType); ok {
		// The universal write obligation gates a write that must satisfy every
		// field a dynamic key may denote, which is correct for a sealed,
		// declared table whose shape is fixed. A mutable/fresh table reached by
		// a dynamic key instead widens: the value domain admits the write by
		// extending the table's map component, leaving the existing fields
		// untouched. Honor that admission here so an inferred table is not gated
		// by the strict heterogeneous-field meet.
		if p.indexTargetSealed(target, point) {
			return expected
		}
		valType := p.AssignmentSourceType(source, point, nil, target.Symbol)
		if widened := value.AdmitIndexedWrite(objType, keyType, valType); widened != nil && !typ.TypeEquals(widened, objType) {
			return nil
		}
		return expected
	}
	return nil
}

// indexTargetSealed reports whether the index target's base denotes a declared,
// annotation-sealed table whose shape a dynamic write must not widen. Mutable
// tables built from a literal or a fresh allocation are not sealed, so a dynamic
// write widens them instead of meeting a heterogeneous-field write obligation.
func (p Projector) indexTargetSealed(target cfg.AssignTarget, point cfg.Point) bool {
	if p.cfg.Inputs == nil {
		return false
	}
	sym := target.BaseSymbol
	if sym == 0 {
		basePath := p.pathOfExpr(target.Base, point)
		sym = basePath.Symbol
	}
	if sym == 0 || !p.cfg.Inputs.AnnotatedVars[sym] {
		return false
	}
	declared := p.cfg.Inputs.DeclaredTypes[sym]
	return !typ.IsRefinableAnnotation(declared)
}

func (p Projector) assignmentTargetFlowWriteType(target cfg.AssignTarget, source ast.Expr, point cfg.Point) typ.Type {
	if p.cfg.Solution == nil {
		return nil
	}
	targetPath := p.indexTargetBasePath(target, point)
	if targetPath.IsEmpty() {
		return nil
	}
	keyPath := p.pathOfExpr(target.Key, point)
	value, ok := p.cfg.Solution.IndexWriteAdmission(flow.IndexWriteQuery{
		Point:     point,
		Target:    targetPath,
		KeySymbol: keyPath.Symbol,
		KeyType:   p.TypeOf(target.Key, point),
		ValuePath: p.pathOfExpr(source, point),
	})
	if !ok || typ.IsAbsentOrUnknown(value) || typ.IsAny(value) {
		return nil
	}
	return value
}

func (p Projector) indexTargetBasePath(target cfg.AssignTarget, point cfg.Point) constraint.Path {
	path := p.pathOfExpr(target.Base, point)
	if !path.IsEmpty() {
		return path
	}
	if target.BaseSymbol == 0 {
		return constraint.Path{}
	}
	return constraint.Path{Root: target.BaseName, Symbol: target.BaseSymbol}
}

// AssignmentTargetDeleteAllowed reports whether assigning nil to target is a
// table deletion instead of an invalid nil write.
func (p Projector) AssignmentTargetDeleteAllowed(target cfg.AssignTarget, point cfg.Point) bool {
	if target.Kind != cfg.TargetIndex || target.Base == nil {
		return false
	}
	objType := p.TypeOf(target.Base, point)
	keyType := typ.Type(nil)
	if target.Key != nil {
		keyType = p.TypeOf(target.Key, point)
	}
	return querycore.IndexDelete(objType, keyType)
}

// ExcludesExprTypeAt reports whether the solved flow product proves declared
// impossible for expr at point.
func (p Projector) ExcludesExprTypeAt(point cfg.Point, expr ast.Expr, declared typ.Type) bool {
	if p.cfg.Solution == nil || expr == nil || declared == nil {
		return false
	}
	path := p.pathOfExpr(expr, point)
	if path.IsEmpty() {
		return false
	}
	return p.cfg.Solution.ExcludesTypeAt(point, path, declared)
}

func (p Projector) exprReferencesSymbol(expr ast.Expr, sym cfg.SymbolID) bool {
	if expr == nil || sym == 0 || p.cfg.Graph == nil {
		return false
	}
	bindings := p.cfg.Bindings
	if bindings == nil {
		bindings = p.cfg.Graph.Bindings()
	}
	if bindings == nil {
		return false
	}
	return provenance.ExprReferencesSymbol(expr, sym, bindings)
}
