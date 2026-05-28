package observation

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/provenance"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
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
	// A stored source type that still carries a type parameter is a generic call
	// synthesized bottom-up without its expected return, so the parameter was never
	// bound. When an expected type is available, re-synthesize the source with it so
	// bidirectional inference instantiates the parameter (e.g. registry.get<T>(): T?
	// assigned to a number? annotation resolves T to number).
	if expected != nil {
		if t, ok := p.assignmentSourceProductType(point, targetSym); ok && !typ.ContainsTypeParam(t) {
			return p.refineAssignmentSourceIndexRead(t, source, point)
		}
		if t, ok := p.assignmentPathSourceType(source, point, targetSym); ok && !typ.ContainsTypeParam(t) {
			return t
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
	}
	if declared != nil {
		return declared, true
	}
	return nil, false
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
