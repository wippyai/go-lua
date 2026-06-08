package observation

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/provenance"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// AssignmentSourceType projects the value read by an assignment source. If the
// source references the target symbol, reads use the point-entry state so
// diagnostics do not inspect the value after the assignment has overwritten it.
func (p Projector) AssignmentSourceType(source ast.Expr, point cfg.Point, expected typ.Type, targetSym cfg.SymbolID) typ.Type {
	if source == nil {
		return nil
	}
	stored, storedOK := p.assignmentSourceProductType(point, targetSym)
	if storedOK {
		stored = p.refineAssignmentSourceIndexRead(stored, source, point)
	}
	path, pathOK := p.assignmentPathSourceType(source, point, targetSym)

	if expected != nil {
		// A stored/path source type that still carries a type parameter is a generic
		// call synthesized bottom-up without its expected return, so the parameter was
		// never bound. When an expected type is available, re-synthesize the source
		// with it so bidirectional inference instantiates the parameter (e.g.
		// registry.get<T>(): T? assigned to a number? annotation resolves T to number).
		if storedOK && typ.ContainsTypeParam(stored) {
			stored = nil
		}
		if pathOK && typ.ContainsTypeParam(path) {
			path = nil
		}
		if selected := flow.SelectAssignmentSourceObservation(flow.AssignmentSourceObservationSelection{
			Stored: stored,
			Path:   path,
		}); !typ.IsAbsentOrUnknown(selected) {
			return p.admitGradualAssignmentSource(selected, source, point, expected)
		}
		return p.assignmentSourceProjector(source, targetSym).TypeOfWithExpected(source, point, expected)
	}

	if selected := flow.SelectAssignmentSourceObservation(flow.AssignmentSourceObservationSelection{
		Stored: stored,
		Path:   path,
	}); !typ.IsAbsentOrUnknown(selected) {
		return selected
	}
	return p.assignmentSourceProjector(source, targetSym).TypeOfWithExpected(source, point, expected)
}

// ReturnSourceType projects a returned expression at a declared-return boundary.
// Path-like returns consume the same normalized path and condition-proof surface
// as assignment sources, but there is no target symbol whose write could
// self-reference the read.
func (p Projector) ReturnSourceType(source ast.Expr, point cfg.Point, expected typ.Type) typ.Type {
	if source == nil {
		return nil
	}
	path, pathOK := p.assignmentPathSourceType(source, point, 0)
	if expected != nil {
		if pathOK && typ.ContainsTypeParam(path) {
			path = nil
			pathOK = false
		}
		if pathOK && !typ.IsAbsentOrUnknown(path) {
			return p.admitGradualAssignmentSource(path, source, point, expected)
		}
		return p.TypeOfWithExpected(source, point, expected)
	}
	if pathOK && !typ.IsAbsentOrUnknown(path) {
		return path
	}
	return p.TypeOfWithExpected(source, point, expected)
}

// admitGradualAssignmentSource is intentionally strict. Earlier revisions used
// this hook to coerce gradual-top `any` to the expected annotation at assignment
// and return boundaries. That made `any` a proof escape hatch. The canonical
// policy keeps `any` as an atom but requires a proof from narrowing, assertion,
// cast, or predicate evidence before a concrete typed write/return is accepted.
func (p Projector) admitGradualAssignmentSource(t typ.Type, source ast.Expr, point cfg.Point, expected typ.Type) typ.Type {
	return t
}

// sourceAnyIsGradualTop reports whether a source observed as `any` is the
// gradual top admissible at a CONSISTENCY boundary (assignment to a typed local,
// return, call argument), rather than an `any` the boundary check must still flag.
//
// The preferred proof is the product-valued path observation boundary, which
// reads the product carrier's evidence axis without splitting root and member
// reads at the caller. That distinguishes a gradual `any` introduced by an
// unannotated source from a strict declared `any`, even though both project to
// typ.Any. Product evidence is authoritative when present; the unannotated-
// parameter root check is only a compatibility fallback for flow surfaces that
// do not yet expose product facts.
//
// This relation gates CONSISTENCY boundaries only. A WRITE into a typed container's
// element slot (a structured index-write target) is a store into invariant
// container state, not a boundary coercion: its element-domain obligation is gated
// by the strict source type (see checkStructuredAssignmentTarget's strict source
// projection).
func (p Projector) sourceAnyIsGradualTop(source ast.Expr, point cfg.Point) bool {
	return p.exprIsGradualTop(source, point)
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
	return p.applyIndexReadProof(t, p.TypeOf(attr.Object, point), attr.Object, attr.Key, p.TypeOf(attr.Key, point), point)
}

func (p Projector) assignmentSourceProjector(source ast.Expr, targetSym cfg.SymbolID) Projector {
	if targetSym != 0 && p.exprReferencesSymbol(source, targetSym) {
		return p.WithPreStateReads()
	}
	return p
}

func (p Projector) assignmentSourceProductType(point cfg.Point, targetSym cfg.SymbolID) (typ.Type, bool) {
	if p.cfg.Inputs == nil || targetSym == 0 {
		return nil, false
	}
	sourceFacts := p.proofs.AssignmentSourceFacts()
	if sourceFacts == nil {
		return nil, false
	}
	for _, assign := range p.cfg.Inputs.Assignments {
		if assign.Point != point || assign.TargetPath.Symbol != targetSym {
			continue
		}
		if assign.Source.Kind == flow.AssignmentSourceStatic && assign.Source.ProjectionKind == flow.AssignmentSourceProjectionNone {
			return nil, false
		}
		t := sourceFacts.AssignmentSourceValueAt(point, assign.TargetPath, assign.Source)
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
	selfReference := targetSym != 0 && p.exprReferencesSymbol(source, targetSym)
	view := flow.PathReadCurrent
	if selfReference {
		view = flow.PathReadPre
	}
	obs := p.proofs.PathObservationFacts(p).ObservePath(flow.PathObservationQuery{
		Point:               point,
		Path:                path,
		View:                view,
		StrictView:          selfReference,
		AllowConditionProof: true,
		PreserveProof:       p.cfg.PreserveProof,
		IndexRead:           p.pathObservationIndexRead(source, point),
	})
	return obs.Type, obs.Resolved()
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
	sealed := p.indexTargetSealed(target, point)
	keyType := typ.Type(nil)
	if target.Key != nil {
		keyType = p.assignmentTargetIndexKeyType(target, point, !sealed)
	}
	if sealed {
		return value.SealedIndexedWriteObligation(objType, keyType)
	}
	if keyType != nil && !sealed {
		valType := p.AssignmentSourceType(source, point, nil, target.Symbol)
		if valType != nil {
			if widened := value.AdmitForeignIndexedWrite(objType, keyType, valType); widened != nil && !typ.TypeEquals(widened, objType) {
				return nil
			}
		}
	}
	if expected, ok := querycore.IndexWrite(objType, keyType); ok {
		return expected
	}
	if expected, ok := querycore.IndexWriteObligation(objType, keyType); ok {
		// The universal write obligation gates a write that must satisfy every
		// field a dynamic key may denote, which is correct for a sealed,
		// declared table whose shape is fixed. A mutable/fresh table reached by
		// a dynamic key instead widens through the value-domain admission law:
		// exact keys add exact members, while broad keys weaken any field they
		// may overwrite and extend the map component. Honor that admission here
		// so an inferred table is not gated by the strict heterogeneous-field
		// meet.
		if sealed {
			return expected
		}
		valType := p.AssignmentSourceType(source, point, nil, target.Symbol)
		if widened := value.AdmitForeignIndexedWrite(objType, keyType, valType); widened != nil && !typ.TypeEquals(widened, objType) {
			return nil
		}
		return expected
	}
	return nil
}

func (p Projector) assignmentTargetIndexKeyType(target cfg.AssignTarget, point cfg.Point, allowConst bool) typ.Type {
	if target.Key == nil {
		return nil
	}
	attr := &ast.AttrGetExpr{Key: target.Key, KeySyntax: ast.AttrKeyIndex}
	constResolver := (func(string) *flow.ConstValue)(nil)
	if allowConst {
		constResolver = p.constResolver(point)
	}
	if seg, ok := flowpath.StaticAttrSegmentWithConst(attr, constResolver); ok {
		switch seg.Kind {
		case constraint.SegmentIndexString:
			return typ.LiteralString(seg.Name)
		case constraint.SegmentIndexInt:
			return typ.LiteralInt(int64(seg.Index))
		case constraint.SegmentField:
			return typ.LiteralString(seg.Name)
		}
	}
	return p.TypeOf(target.Key, point)
}

// indexTargetSealed reports whether the index target's base denotes a declared,
// annotation-sealed table whose shape a dynamic write must not widen. Mutable
// tables built from a literal or a fresh allocation are not sealed, so a dynamic
// write widens them instead of meeting a heterogeneous-field write obligation.
func (p Projector) indexTargetSealed(target cfg.AssignTarget, point cfg.Point) bool {
	basePath := p.indexTargetBasePath(target, point)
	if basePath.IsEmpty() || basePath.Symbol == 0 {
		return false
	}
	if p.cfg.Facts != nil {
		return flow.AnnotatedDeclaredPathSealed(p.cfg.Facts, point, basePath)
	}
	if p.cfg.Inputs == nil || !p.cfg.Inputs.AnnotatedVars.Contains(basePath.Symbol) {
		return false
	}
	declared := DeclaredPathType(p.cfg.Inputs.DeclaredTypes, basePath)
	return declared != nil && !typ.IsAbsentOrUnknown(declared) && !typ.IsRefinableAnnotation(declared)
}

func (p Projector) assignmentTargetFlowWriteType(target cfg.AssignTarget, source ast.Expr, point cfg.Point) typ.Type {
	targetPath := p.indexTargetBasePath(target, point)
	if targetPath.IsEmpty() {
		return nil
	}
	keyPath := p.pathOfExpr(target.Key, point)
	valuePath := p.pathOfExpr(source, point)
	admitted, ok := p.proofs.IndexReadPointFacts(point, flow.PathReadPost).IndexWriteAdmission(flow.IndexWritePathQuery{
		Target:       targetPath,
		KeyPath:      keyPath,
		HasKeyPath:   !keyPath.IsEmpty(),
		KeyValue:     product.FromType(p.TypeOf(target.Key, point)),
		ValuePath:    valuePath,
		HasValuePath: !valuePath.IsEmpty(),
	})
	value := product.ProjectValueOrUnknown(admitted)
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
	if p.cfg.Flow == nil || expr == nil || declared == nil {
		return false
	}
	path := p.pathOfExpr(expr, point)
	if path.IsEmpty() {
		return false
	}
	return p.cfg.Flow.ExcludesTypeAt(point, path, declared)
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
